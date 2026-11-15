package middleware

import (
	"strconv"

	"elearning-sbt/database"
	"elearning-sbt/models"
	"elearning-sbt/services"

	"github.com/gofiber/fiber/v2"
)

// UserContextMiddleware attaches the resolved UserID to c.Locals for all API requests
func UserContextMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := ResolveUserID(c)
		c.Locals("userID", userID)
		return c.Next()
	}
}

// AccessGuardMiddleware enforces sequential lesson access (SortOrder prerequisites)
func AccessGuardMiddleware(courseService *services.CourseService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		lessonIDStr := c.Params("lessonId")
		if lessonIDStr == "" {
			return c.Next()
		}

		lessonIDUint, err := strconv.ParseUint(lessonIDStr, 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"message": "Invalid lesson ID format",
			})
		}
		lessonID := uint(lessonIDUint)

		// Resolve User ID from Header, Cookie, or Query
		userID := ResolveUserID(c)
		c.Locals("userID", userID)

		// Check sequential access permission
		allowed, reason, err := courseService.CheckLessonAccess(userID, lessonID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "Error validating lesson access",
				"error":   err.Error(),
			})
		}

		if !allowed {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"error":   "ACCESS_DENIED_PREREQUISITE",
				"message": reason,
			})
		}

		return c.Next()
	}
}

// ResolveUserID helper resolves User ID from request headers or wallet address
func ResolveUserID(c *fiber.Ctx) uint {
	// 1. Direct User ID header
	if uidStr := c.Get("X-User-ID"); uidStr != "" {
		if uid, err := strconv.ParseUint(uidStr, 10, 32); err == nil {
			return uint(uid)
		}
	}

	// 2. Query param
	if uidStr := c.Query("user_id"); uidStr != "" {
		if uid, err := strconv.ParseUint(uidStr, 10, 32); err == nil {
			return uint(uid)
		}
	}

	// 3. Wallet Address lookup
	wallet := c.Get("X-Wallet-Address")
	if wallet == "" {
		wallet = c.Query("wallet")
	}

	if wallet != "" {
		var user models.User
		if err := database.DB.Where("wallet_address = ?", wallet).First(&user).Error; err == nil {
			return user.ID
		}
	}

	// Default fallback to Demo Learner (ID: 2) if in demo mode and no header
	return 2
}
