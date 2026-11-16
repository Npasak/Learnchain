package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"

	"github.com/gofiber/fiber/v2"
)

// SignatoryHandler manages certificate signatories (added by Manager, selected by Lesson Creator).
type SignatoryHandler struct{}

func NewSignatoryHandler() *SignatoryHandler {
	return &SignatoryHandler{}
}

func isManagerOrAdmin(c *fiber.Ctx) bool {
	userID := middleware.ResolveUserID(c)
	if userID == SecretAdminUserID {
		return true
	}
	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return false
	}
	return strings.EqualFold(user.Role, "manager")
}

// ListSignatories returns all active certificate signatories (accessible to Manager, Creator, and Learner for display).
func (h *SignatoryHandler) ListSignatories(c *fiber.Ctx) error {
	var list []models.Signatory
	if err := database.DB.Order("id asc").Find(&list).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่สามารถโหลดรายชื่อผู้ลงนามได้",
		})
	}
	return c.JSON(fiber.Map{
		"success":     true,
		"signatories": list,
	})
}

// UploadSignatureImage saves an uploaded signature image into ./covers/ and returns its /covers/ URL.
func (h *SignatoryHandler) UploadSignatureImage(c *fiber.Ctx) error {
	if !isManagerOrAdmin(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "เฉพาะผู้จัดการเท่านั้นที่สามารถอัปโหลดลายเซ็นได้",
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่พบไฟล์รูปลายเซ็น",
		})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".svg":
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "รองรับเฉพาะไฟล์รูปภาพ (.png, .jpg, .webp, .svg)",
		})
	}

	_ = os.MkdirAll("./covers", 0755)
	filename := fmt.Sprintf("sig_%d%s", time.Now().UnixNano(), ext)
	savePath := filepath.Join("./covers", filename)
	if err := c.SaveFile(file, savePath); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่สามารถบันทึกรูปลายเซ็นได้",
		})
	}

	return c.JSON(fiber.Map{
		"success":       true,
		"signature_url": "/covers/" + filename,
	})
}

type signatoryRequest struct {
	FullName     string `json:"full_name"`
	Position     string `json:"position"`
	SignatureURL string `json:"signature_url"`
}

// CreateSignatory adds a new signatory (Manager only).
func (h *SignatoryHandler) CreateSignatory(c *fiber.Ctx) error {
	if !isManagerOrAdmin(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "เฉพาะผู้จัดการเท่านั้นที่สามารถเพิ่มผู้ลงนามได้",
		})
	}

	var req signatoryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "ข้อมูลไม่ถูกต้อง",
		})
	}

	req.FullName = strings.TrimSpace(req.FullName)
	req.Position = strings.TrimSpace(req.Position)
	req.SignatureURL = strings.TrimSpace(req.SignatureURL)

	if req.FullName == "" || req.Position == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "กรุณาระบุชื่อ-นามสกุล และตำแหน่ง",
		})
	}

	sig := models.Signatory{
		FullName:     req.FullName,
		Position:     req.Position,
		SignatureURL: req.SignatureURL,
		CreatedByID:  middleware.ResolveUserID(c),
	}
	if err := database.DB.Create(&sig).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่สามารถบันทึกผู้ลงนามได้",
		})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"signatory": sig,
	})
}

// UpdateSignatory updates an existing signatory (Manager only).
func (h *SignatoryHandler) UpdateSignatory(c *fiber.Ctx) error {
	if !isManagerOrAdmin(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "เฉพาะผู้จัดการเท่านั้นที่สามารถแก้ไขผู้ลงนามได้",
		})
	}

	var sig models.Signatory
	if err := database.DB.First(&sig, c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่พบข้อมูลผู้ลงนาม",
		})
	}

	var req signatoryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "ข้อมูลไม่ถูกต้อง",
		})
	}

	if full := strings.TrimSpace(req.FullName); full != "" {
		sig.FullName = full
	}
	if pos := strings.TrimSpace(req.Position); pos != "" {
		sig.Position = pos
	}
	if url := strings.TrimSpace(req.SignatureURL); url != "" {
		sig.SignatureURL = url
	}

	if err := database.DB.Save(&sig).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่สามารถอัปเดตข้อมูลผู้ลงนามได้",
		})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"signatory": sig,
	})
}

// DeleteSignatory soft-deletes a signatory (Manager only).
func (h *SignatoryHandler) DeleteSignatory(c *fiber.Ctx) error {
	if !isManagerOrAdmin(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "เฉพาะผู้จัดการเท่านั้นที่สามารถลบผู้ลงนามได้",
		})
	}

	if err := database.DB.Delete(&models.Signatory{}, c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่สามารถลบผู้ลงนามได้",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
	})
}
