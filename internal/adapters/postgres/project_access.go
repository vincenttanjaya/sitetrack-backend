package postgres

import (
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

// projectAccess uses the current membership. Mutations acquire the common project
// row lock first and then recheck membership using a fresh READ COMMITTED snapshot.
func projectAccess(db *gorm.DB, actor domain.Actor, id string, lock bool) (isAdmin bool, err error) {
	if lock {
		var existing string
		err := db.Raw(`SELECT p.id FROM projects p WHERE p.id = ?
   AND (? OR EXISTS (SELECT 1 FROM project_members WHERE project_id = p.id AND user_id = ?))
   FOR UPDATE OF p`, id, actor.Role == domain.RoleSuperAdmin, actor.UserID).Row().Scan(&existing)
		if err != nil {
			return false, dbError(err)
		}
	}
	err = db.Raw(`SELECT COALESCE(m.is_admin, false)
  FROM projects p LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = ?
  WHERE p.id = ? AND (? OR m.user_id IS NOT NULL)`, actor.UserID, id, actor.Role == domain.RoleSuperAdmin).Row().Scan(&isAdmin)
	return isAdmin, dbError(err)
}
