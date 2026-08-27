package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/handler"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
)

func TestCreateNotification(t *testing.T) {
	db := setupTestDB(t)
	user, _ := createTestUser(t, db, "notif-user1", "member")

	handler.CreateNotification(db, user.ID, "test", "Test Title", "Test body", "/test-link")

	var count int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ?", user.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 notification, got %d", count)
	}
}

func TestListNotifications(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "notif-list1", "member")

	handler.CreateNotification(db, user.ID, "test", "Notif 1", "body 1", "")
	handler.CreateNotification(db, user.ID, "test", "Notif 2", "body 2", "")
	handler.CreateNotification(db, user.ID, "test", "Notif 3", "body 3", "")

	r := authedRequest("GET", "/api/v1/notifications", nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/notifications", middleware.AuthRequired(db, handler.ListNotifications(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	items, ok := result["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(items))
	}
}

func TestFilterUnreadNotifications(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "notif-unread1", "member")

	handler.CreateNotification(db, user.ID, "test", "Unread 1", "", "")
	handler.CreateNotification(db, user.ID, "test", "Unread 2", "", "")

	// Mark one as read.
	var firstID string
	db.QueryRow("SELECT id FROM notifications WHERE user_id = ? ORDER BY id ASC LIMIT 1", user.ID).Scan(&firstID)
	db.Exec("UPDATE notifications SET read_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?", firstID)

	// Filter unread only.
	r := authedRequest("GET", "/api/v1/notifications?unread=true", nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/notifications", middleware.AuthRequired(db, handler.ListNotifications(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	result := decodeJSON(t, w)
	items := result["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 unread notification, got %d", len(items))
	}
}

func TestMarkNotificationRead(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "notif-read1", "member")

	handler.CreateNotification(db, user.ID, "test", "To read", "", "")

	var notifID string
	db.QueryRow("SELECT id FROM notifications WHERE user_id = ?", user.ID).Scan(&notifID)

	r := authedRequest("PATCH", "/api/v1/notifications/"+notifID+"/read", nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/notifications/{id}/read", middleware.AuthRequired(db, handler.MarkNotificationRead(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's marked.
	var readAt *string
	db.QueryRow("SELECT read_at FROM notifications WHERE id = ?", notifID).Scan(&readAt)
	if readAt == nil {
		t.Fatal("expected read_at to be set")
	}
}

func TestMarkNotificationRead_WrongUser(t *testing.T) {
	db := setupTestDB(t)
	user1, _ := createTestUser(t, db, "notif-owner1", "member")
	_, user2Token := createTestUser(t, db, "notif-other1", "member")

	handler.CreateNotification(db, user1.ID, "test", "Private notif", "", "")

	var notifID string
	db.QueryRow("SELECT id FROM notifications WHERE user_id = ?", user1.ID).Scan(&notifID)

	// user2 tries to mark user1's notification as read.
	r := authedRequest("PATCH", "/api/v1/notifications/"+notifID+"/read", nil, user2Token)
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/notifications/{id}/read", middleware.AuthRequired(db, handler.MarkNotificationRead(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMarkAllNotificationsRead(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "notif-markall1", "member")

	handler.CreateNotification(db, user.ID, "test", "Notif A", "", "")
	handler.CreateNotification(db, user.ID, "test", "Notif B", "", "")
	handler.CreateNotification(db, user.ID, "test", "Notif C", "", "")

	r := authedRequest("POST", "/api/v1/notifications/read-all", nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/notifications/read-all", middleware.AuthRequired(db, handler.MarkAllNotificationsRead(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	if result["updated"].(float64) != 3 {
		t.Fatalf("expected 3 updated, got %v", result["updated"])
	}

	// Verify all are read.
	var unreadCount int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL", user.ID).Scan(&unreadCount)
	if unreadCount != 0 {
		t.Fatalf("expected 0 unread, got %d", unreadCount)
	}
}

func TestDeleteNotification(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "notif-del1", "member")

	handler.CreateNotification(db, user.ID, "test", "To delete", "", "")

	var notifID string
	db.QueryRow("SELECT id FROM notifications WHERE user_id = ?", user.ID).Scan(&notifID)

	r := authedRequest("DELETE", "/api/v1/notifications/"+notifID, nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/notifications/{id}", middleware.AuthRequired(db, handler.DeleteNotification(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ?", user.ID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 notifications, got %d", count)
	}
}

func TestDeleteNotification_WrongUser(t *testing.T) {
	db := setupTestDB(t)
	user1, _ := createTestUser(t, db, "notif-delowner1", "member")
	_, user2Token := createTestUser(t, db, "notif-delother1", "member")

	handler.CreateNotification(db, user1.ID, "test", "Private notif", "", "")

	var notifID string
	db.QueryRow("SELECT id FROM notifications WHERE user_id = ?", user1.ID).Scan(&notifID)

	// user2 tries to delete user1's notification.
	r := authedRequest("DELETE", "/api/v1/notifications/"+notifID, nil, user2Token)
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/notifications/{id}", middleware.AuthRequired(db, handler.DeleteNotification(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ?", user1.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected notification to survive, got %d", count)
	}
}

func TestClearNotifications(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "notif-clear1", "member")
	other, _ := createTestUser(t, db, "notif-clearother1", "member")

	handler.CreateNotification(db, user.ID, "test", "N1", "", "")
	handler.CreateNotification(db, user.ID, "test", "N2", "", "")
	handler.CreateNotification(db, user.ID, "test", "N3", "", "")
	handler.CreateNotification(db, other.ID, "test", "Keep me", "", "")

	r := authedRequest("DELETE", "/api/v1/notifications", nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/notifications", middleware.AuthRequired(db, handler.ClearNotifications(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	if result["deleted"].(float64) != 3 {
		t.Fatalf("expected 3 deleted, got %v", result["deleted"])
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ?", user.ID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 notifications for user, got %d", count)
	}

	// Other user's notifications are untouched.
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ?", other.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected other user's notification to survive, got %d", count)
	}
}

func TestNotificationCount(t *testing.T) {
	db := setupTestDB(t)
	user, userToken := createTestUser(t, db, "notif-count1", "member")

	handler.CreateNotification(db, user.ID, "test", "N1", "", "")
	handler.CreateNotification(db, user.ID, "test", "N2", "", "")

	r := authedRequest("GET", "/api/v1/notifications/count", nil, userToken)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/notifications/count", middleware.AuthRequired(db, handler.NotificationCount(db)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	result := decodeJSON(t, w)
	if result["unread"].(float64) != 2 {
		t.Fatalf("expected 2 unread, got %v", result["unread"])
	}
}
