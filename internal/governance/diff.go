package governance

import (
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffResult represents the diff between two versions of a document.
type DiffResult struct {
	FromSHA  string     `json:"from_sha"`
	ToSHA    string     `json:"to_sha"`
	Filename string     `json:"filename"`
	Hunks    []DiffHunk `json:"hunks"`
	Unified  string     `json:"unified"` // unified diff format
}

// DiffHunk represents a contiguous block of changes.
type DiffHunk struct {
	Type string `json:"type"` // "equal", "insert", "delete"
	Text string `json:"text"`
}

// GetDiff computes the diff between two versions of a file.
func GetDiff(dataDir, nodeID, filename, fromSHA, toSHA string) (*DiffResult, error) {
	fromContent, err := GetDocumentAtVersion(dataDir, nodeID, filename, fromSHA)
	if err != nil {
		return nil, fmt.Errorf("get from version: %w", err)
	}

	toContent, err := GetDocumentAtVersion(dataDir, nodeID, filename, toSHA)
	if err != nil {
		return nil, fmt.Errorf("get to version: %w", err)
	}

	return DiffStrings(fromContent, toContent, fromSHA, toSHA, filename), nil
}

// DiffStrings computes a word-level diff between two strings.
// Used both for version comparison and amendment proposal preview.
func DiffStrings(oldText, newText, fromSHA, toSHA, filename string) *DiffResult {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldText, newText, true)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var hunks []DiffHunk
	for _, d := range diffs {
		var hunkType string
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			hunkType = "equal"
		case diffmatchpatch.DiffInsert:
			hunkType = "insert"
		case diffmatchpatch.DiffDelete:
			hunkType = "delete"
		}
		hunks = append(hunks, DiffHunk{
			Type: hunkType,
			Text: d.Text,
		})
	}

	// Generate unified diff format
	patches := dmp.PatchMake(oldText, diffs)
	unified := dmp.PatchToText(patches)

	return &DiffResult{
		FromSHA:  fromSHA,
		ToSHA:    toSHA,
		Filename: filename,
		Hunks:    hunks,
		Unified:  unified,
	}
}

// GetDiffFromBranch computes the diff between main and a branch for a specific file.
// Used to show what an amendment proposal would change.
func GetDiffFromBranch(dataDir, nodeID, branchName, filename string) (*DiffResult, error) {
	repo, err := openBare(NodeRepoPath(dataDir, nodeID))
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	// Get main content
	mainRef, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("main head: %w", err)
	}
	mainCommit, err := repo.CommitObject(mainRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("main commit: %w", err)
	}
	mainFile, err := mainCommit.File(filename)
	if err != nil {
		return nil, fmt.Errorf("main file: %w", err)
	}
	mainContent, err := mainFile.Contents()
	if err != nil {
		return nil, fmt.Errorf("main content: %w", err)
	}

	// Get branch content
	branchRefName := plumbing.NewBranchReferenceName(branchName)
	branchRef, err := repo.Reference(branchRefName, true)
	if err != nil {
		return nil, fmt.Errorf("branch %s: %w", branchName, err)
	}
	branchCommit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("branch commit: %w", err)
	}
	branchFile, err := branchCommit.File(filename)
	if err != nil {
		return nil, fmt.Errorf("branch file: %w", err)
	}
	branchContent, err := branchFile.Contents()
	if err != nil {
		return nil, fmt.Errorf("branch content: %w", err)
	}

	return DiffStrings(mainContent, branchContent, mainRef.Hash().String(), branchRef.Hash().String(), filename), nil
}

// HasChanges returns true if the file content on the branch differs from main.
func HasChanges(dataDir, nodeID, branchName, filename string) (bool, error) {
	diff, err := GetDiffFromBranch(dataDir, nodeID, branchName, filename)
	if err != nil {
		return false, err
	}
	for _, h := range diff.Hunks {
		if h.Type != "equal" {
			return true, nil
		}
	}
	return false, nil
}

