// Package governance manages git-backed governance documents for Patchwork.
// Each patch gets a bare git repo. The instance has a base repo that patches fork from.
//
// All operations are performed via go-git (pure Go, no external git binary
// needed). This is a hard requirement: the production image is distroless and
// ships no git binary. In particular, never clone from a local filesystem path
// — go-git's file transport execs `git upload-pack` under the hood. Repos are
// created by writing git objects directly (see initBareRepoWithFiles); the
// gitless guarantee is enforced by TestGitlessRuntime.
package governance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// dataDir holds the base data directory (set at startup via SetDataDir).
var dataDir string

// SetDataDir stores the base data directory so governance functions can locate repos.
func SetDataDir(dir string) { dataDir = dir }

// GetDataDir returns the base data directory.
func GetDataDir() string { return dataDir }

func init() {
	// Register HTTP transport for go-git (needed for clone/fetch over HTTP)
	client.InstallProtocol("http", githttp.NewClient(nil))
	client.InstallProtocol("https", githttp.NewClient(nil))
}

// DocInfo describes a governance document (file) in a repo.
type DocInfo struct {
	Filename string `json:"filename"`
	Title    string `json:"title"` // derived from first heading or filename
	Size     int64  `json:"size"`
}

// CommitInfo describes a single commit in the governance repo history.
type CommitInfo struct {
	SHA           string    `json:"sha"`
	Message       string    `json:"message"`
	AuthorName    string    `json:"author_name"`
	AuthorEmail   string    `json:"author_email"`
	Date          time.Time `json:"date"`
	VersionNumber int       `json:"version_number"` // 1-indexed, computed from position in history
}

// repoPath returns the filesystem path for a governance repo.
func repoPath(dataDir, repoID string) string {
	return filepath.Join(dataDir, "governance", repoID+".git")
}

// InstanceRepoPath returns the path to the instance-level governance repo.
func InstanceRepoPath(dataDir string) string {
	return repoPath(dataDir, "instance")
}

// NodeRepoPath returns the path to a node's governance repo.
func NodeRepoPath(dataDir, nodeID string) string {
	return repoPath(dataDir, nodeID)
}

// InitInstanceRepo creates the instance-level governance repo with default documents.
// If the repo already exists, this is a no-op.
func InitInstanceRepo(dataDir string) error {
	path := InstanceRepoPath(dataDir)
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	if err := initBareRepoWithFiles(path, defaultInstanceFiles(), "Initial governance baseline"); err != nil {
		return fmt.Errorf("init instance repo: %w", err)
	}
	return nil
}

// ForkForNode creates a node's governance repo from the instance repo using a template.
// The template parameter selects which governance template to use (e.g., "casual", "collaborative").
// If template is empty, defaults to "casual". If the repo already exists, this is a no-op.
func ForkForNode(dataDir, nodeID, template string) error {
	nodePath := NodeRepoPath(dataDir, nodeID)
	if _, err := os.Stat(nodePath); err == nil {
		return nil // already exists
	}

	if template == "" {
		template = "casual"
	}

	// Build the node's files: community-standards.md (lining) + template files.
	files := map[string]string{
		"community-standards.md": defaultInstanceFiles()["community-standards.md"],
	}
	for filename, content := range templateFiles(template) {
		files[filename] = content
	}

	// If no template files found (invalid template name), fall back to casual.
	if len(files) == 1 {
		for filename, content := range templateFiles("casual") {
			files[filename] = content
		}
	}

	msg := fmt.Sprintf("Initialize governance (%s template)", template)
	if err := initBareRepoWithFiles(nodePath, files, msg); err != nil {
		return fmt.Errorf("fork for node %s: %w", nodeID, err)
	}
	return nil
}

// initBareRepoWithFiles creates a bare repo at path whose main branch holds a
// single commit containing the given files (paths may be nested, e.g.
// "templates/casual/governance-rules.json"). The repo is built by writing git
// objects directly — no worktree, no clone, no git binary. On failure the
// half-created repo is removed so a retry starts clean.
func initBareRepoWithFiles(path string, files map[string]string, message string) error {
	repo, err := git.PlainInit(path, true)
	if err != nil {
		return fmt.Errorf("init bare repo: %w", err)
	}

	fail := func(err error) error {
		os.RemoveAll(path)
		return err
	}

	treeHash, err := buildTreeFromFiles(repo, files)
	if err != nil {
		return fail(err)
	}

	sig := &object.Signature{
		Name:  "Patchwork System",
		Email: "system@patchwork.local",
		When:  time.Now(),
	}
	commitHash, err := createCommit(repo, treeHash, nil, message, sig)
	if err != nil {
		return fail(err)
	}

	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), commitHash)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		return fail(fmt.Errorf("set main ref: %w", err))
	}
	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))
	if err := repo.Storer.SetReference(headRef); err != nil {
		return fail(fmt.Errorf("set HEAD: %w", err))
	}
	return nil
}

// buildTreeFromFiles stores the files as blobs plus nested tree objects and
// returns the root tree hash. File paths use "/" separators.
func buildTreeFromFiles(repo *git.Repository, files map[string]string) (plumbing.Hash, error) {
	root := newTreeDir()
	for name, content := range files {
		blobHash, err := storeBlob(repo, content)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		root.insert(strings.Split(name, "/"), blobHash)
	}
	return root.write(repo)
}

// treeDir is an in-memory directory used to assemble nested git trees.
type treeDir struct {
	blobs map[string]plumbing.Hash
	dirs  map[string]*treeDir
}

func newTreeDir() *treeDir {
	return &treeDir{blobs: map[string]plumbing.Hash{}, dirs: map[string]*treeDir{}}
}

func (d *treeDir) insert(parts []string, blobHash plumbing.Hash) {
	if len(parts) == 1 {
		d.blobs[parts[0]] = blobHash
		return
	}
	sub, ok := d.dirs[parts[0]]
	if !ok {
		sub = newTreeDir()
		d.dirs[parts[0]] = sub
	}
	sub.insert(parts[1:], blobHash)
}

// write encodes this directory (subdirectories first) as git tree objects and
// returns this directory's tree hash.
func (d *treeDir) write(repo *git.Repository) (plumbing.Hash, error) {
	var entries []object.TreeEntry
	for name, hash := range d.blobs {
		entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: hash})
	}
	for name, sub := range d.dirs {
		hash, err := sub.write(repo)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Dir, Hash: hash})
	}

	// Git sorts tree entries as if directory names had a trailing "/".
	sort.Slice(entries, func(i, j int) bool {
		return treeSortKey(entries[i]) < treeSortKey(entries[j])
	})

	treeObj := repo.Storer.NewEncodedObject()
	treeObj.SetType(plumbing.TreeObject)
	if err := (&object.Tree{Entries: entries}).Encode(treeObj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}
	hash, err := repo.Storer.SetEncodedObject(treeObj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store tree: %w", err)
	}
	return hash, nil
}

func treeSortKey(e object.TreeEntry) string {
	if e.Mode == filemode.Dir {
		return e.Name + "/"
	}
	return e.Name
}

// openBare opens a bare git repo.
func openBare(path string) (*git.Repository, error) {
	return git.PlainOpen(path)
}

// ListDocuments returns all files in the main branch of a node's governance repo.
func ListDocuments(dataDir, nodeID string) ([]DocInfo, error) {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return nil, nil // empty repo
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("head commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("tree: %w", err)
	}

	var docs []DocInfo
	tree.Files().ForEach(func(f *object.File) error {
		// List .md files (governance documents) and governance-rules.json
		if strings.HasSuffix(f.Name, ".md") || f.Name == "governance-rules.json" {
			title := titleFromFilename(f.Name)
			docs = append(docs, DocInfo{
				Filename: f.Name,
				Title:    title,
				Size:     f.Size,
			})
		}
		return nil
	})

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Filename < docs[j].Filename
	})

	return docs, nil
}

// GetDocument returns the current content of a file from the main branch.
func GetDocument(dataDir, nodeID, filename string) (string, error) {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("head: %w", err)
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	file, err := commit.File(filename)
	if err != nil {
		return "", fmt.Errorf("file %s not found: %w", filename, err)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filename, err)
	}

	return content, nil
}

// GetDocumentAtVersion returns the content of a file at a specific commit SHA.
func GetDocumentAtVersion(dataDir, nodeID, filename, commitSHA string) (string, error) {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	hash := plumbing.NewHash(commitSHA)
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return "", fmt.Errorf("commit %s: %w", commitSHA, err)
	}

	file, err := commit.File(filename)
	if err != nil {
		return "", fmt.Errorf("file %s at %s: %w", filename, commitSHA, err)
	}

	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}

	return content, nil
}

// GetHistory returns the commit log for a specific file (most recent first).
func GetHistory(dataDir, nodeID, filename string) ([]CommitInfo, error) {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return nil, nil // empty repo
	}

	logIter, err := repo.Log(&git.LogOptions{
		From:     ref.Hash(),
		FileName: &filename,
		Order:    git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	defer logIter.Close()

	var history []CommitInfo
	logIter.ForEach(func(c *object.Commit) error {
		history = append(history, CommitInfo{
			SHA:         c.Hash.String(),
			Message:     strings.TrimSpace(c.Message),
			AuthorName:  c.Author.Name,
			AuthorEmail: c.Author.Email,
			Date:        c.Author.When,
		})
		return nil
	})

	// Number versions (oldest = 1)
	for i := range history {
		history[i].VersionNumber = len(history) - i
	}

	return history, nil
}

// DirectEdit commits a change directly to the main branch.
// Used for admin edits that don't go through the proposal process.
func DirectEdit(dataDir, nodeID, filename, newContent, authorName, authorEmail, message string) (string, error) {
	repoPath := NodeRepoPath(dataDir, nodeID)

	// For bare repos, we need to manipulate objects directly.
	repo, err := openBare(repoPath)
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	// Get current HEAD
	ref, err := repo.Head()
	if err != nil {
		// Empty repo — create first commit
		return createFirstCommit(repo, filename, newContent, authorName, authorEmail, message)
	}

	parentCommit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("parent commit: %w", err)
	}

	parentTree, err := parentCommit.Tree()
	if err != nil {
		return "", fmt.Errorf("parent tree: %w", err)
	}

	// Create new blob
	blobHash, err := storeBlob(repo, newContent)
	if err != nil {
		return "", err
	}

	// Build new tree with the updated file
	newTree, err := updateTree(repo, parentTree, filename, blobHash)
	if err != nil {
		return "", err
	}

	// Create commit
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}
	commitHash, err := createCommit(repo, newTree, []*object.Commit{parentCommit}, message, sig)
	if err != nil {
		return "", err
	}

	// Update HEAD
	refName := ref.Name()
	newRef := plumbing.NewHashReference(refName, commitHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("update ref: %w", err)
	}

	return commitHash.String(), nil
}

// CreateBranch creates a new branch with proposed changes to a file.
// Returns the commit SHA of the new branch head.
func CreateBranch(dataDir, nodeID, branchName, filename, newContent, authorName, authorEmail, message string) (string, error) {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	// Get current HEAD
	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("head: %w", err)
	}

	parentCommit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("parent: %w", err)
	}

	parentTree, err := parentCommit.Tree()
	if err != nil {
		return "", fmt.Errorf("tree: %w", err)
	}

	// Create blob + updated tree
	blobHash, err := storeBlob(repo, newContent)
	if err != nil {
		return "", err
	}

	newTree, err := updateTree(repo, parentTree, filename, blobHash)
	if err != nil {
		return "", err
	}

	// Create commit on the branch
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}
	commitHash, err := createCommit(repo, newTree, []*object.Commit{parentCommit}, message, sig)
	if err != nil {
		return "", err
	}

	// Create branch reference
	branchRef := plumbing.NewHashReference(
		plumbing.NewBranchReferenceName(branchName),
		commitHash,
	)
	if err := repo.Storer.SetReference(branchRef); err != nil {
		return "", fmt.Errorf("create branch: %w", err)
	}

	return commitHash.String(), nil
}

// MergeBranch merges a branch into the main branch (fast-forward or merge commit).
// Returns the resulting commit SHA.
func MergeBranch(dataDir, nodeID, branchName, authorName, authorEmail string) (string, error) {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	// Get main branch HEAD
	mainRef, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("main head: %w", err)
	}
	mainCommit, err := repo.CommitObject(mainRef.Hash())
	if err != nil {
		return "", fmt.Errorf("main commit: %w", err)
	}

	// Get branch HEAD
	branchRefName := plumbing.NewBranchReferenceName(branchName)
	branchRef, err := repo.Reference(branchRefName, true)
	if err != nil {
		return "", fmt.Errorf("branch %s not found: %w", branchName, err)
	}
	branchCommit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		return "", fmt.Errorf("branch commit: %w", err)
	}

	// Check if fast-forward is possible (main is ancestor of branch)
	isAncestor, err := mainCommit.IsAncestor(branchCommit)
	if err != nil {
		return "", fmt.Errorf("ancestor check: %w", err)
	}

	var resultHash plumbing.Hash

	if isAncestor {
		// Fast-forward: just move the main ref to the branch commit
		resultHash = branchRef.Hash()
	} else {
		// Create a merge commit using the branch's tree (branch wins)
		sig := &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		}
		msg := fmt.Sprintf("Merge amendment: %s", branchName)
		resultHash, err = createCommit(repo, branchCommit.TreeHash, []*object.Commit{mainCommit, branchCommit}, msg, sig)
		if err != nil {
			return "", err
		}
	}

	// Update main branch reference
	newRef := plumbing.NewHashReference(mainRef.Name(), resultHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("update main: %w", err)
	}

	// Delete the merged branch
	repo.Storer.RemoveReference(branchRefName)

	return resultHash.String(), nil
}

// DeleteBranch removes a branch reference.
func DeleteBranch(dataDir, nodeID, branchName string) error {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	branchRefName := plumbing.NewBranchReferenceName(branchName)
	return repo.Storer.RemoveReference(branchRefName)
}

// --- Internal helpers ---

func titleFromFilename(filename string) string {
	name := filepath.Base(filename)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	// Title case
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func storeBlob(repo *git.Repository, content string) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("blob writer: %w", err)
	}
	w.Write([]byte(content))
	w.Close()
	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store blob: %w", err)
	}
	return hash, nil
}

func updateTree(repo *git.Repository, parentTree *object.Tree, filename string, blobHash plumbing.Hash) (plumbing.Hash, error) {
	// Build new tree entries from parent, replacing the target file
	var entries []object.TreeEntry
	found := false
	for _, entry := range parentTree.Entries {
		if entry.Name == filename {
			entries = append(entries, object.TreeEntry{
				Name: filename,
				Mode: 0100644,
				Hash: blobHash,
			})
			found = true
		} else {
			entries = append(entries, entry)
		}
	}
	if !found {
		entries = append(entries, object.TreeEntry{
			Name: filename,
			Mode: 0100644,
			Hash: blobHash,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	// Encode tree object
	treeObj := repo.Storer.NewEncodedObject()
	treeObj.SetType(plumbing.TreeObject)
	newTree := &object.Tree{Entries: entries}
	err := newTree.Encode(treeObj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}
	treeHash, err := repo.Storer.SetEncodedObject(treeObj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store tree: %w", err)
	}
	return treeHash, nil
}

func createCommit(repo *git.Repository, treeHash plumbing.Hash, parents []*object.Commit, message string, sig *object.Signature) (plumbing.Hash, error) {
	commitObj := repo.Storer.NewEncodedObject()
	commitObj.SetType(plumbing.CommitObject)

	var parentHashes []plumbing.Hash
	for _, p := range parents {
		parentHashes = append(parentHashes, p.Hash)
	}

	commit := &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      message,
		TreeHash:     treeHash,
		ParentHashes: parentHashes,
	}
	err := commit.Encode(commitObj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode commit: %w", err)
	}
	hash, err := repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store commit: %w", err)
	}
	return hash, nil
}

func createFirstCommit(repo *git.Repository, filename, content, authorName, authorEmail, message string) (string, error) {
	blobHash, err := storeBlob(repo, content)
	if err != nil {
		return "", err
	}

	entries := []object.TreeEntry{{
		Name: filename,
		Mode: 0100644,
		Hash: blobHash,
	}}

	treeObj := repo.Storer.NewEncodedObject()
	treeObj.SetType(plumbing.TreeObject)
	tree := &object.Tree{Entries: entries}
	tree.Encode(treeObj)
	treeHash, err := repo.Storer.SetEncodedObject(treeObj)
	if err != nil {
		return "", fmt.Errorf("store tree: %w", err)
	}

	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}
	commitHash, err := createCommit(repo, treeHash, nil, message, sig)
	if err != nil {
		return "", err
	}

	// Set HEAD → refs/heads/main
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), commitHash)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		return "", fmt.Errorf("set main ref: %w", err)
	}
	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))
	if err := repo.Storer.SetReference(headRef); err != nil {
		return "", fmt.Errorf("set HEAD: %w", err)
	}

	return commitHash.String(), nil
}
