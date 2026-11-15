package handlers

import (
	"fmt"
	"strconv"
	"time"

	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"
	"elearning-sbt/services"

	"github.com/gofiber/fiber/v2"
)

type CertificateHandler struct {
	courseService     *services.CourseService
	blockchainService *services.BlockchainService
	emailService      *services.EmailService
}

func NewCertificateHandler(
	cs *services.CourseService,
	bs *services.BlockchainService,
	es *services.EmailService,
) *CertificateHandler {
	return &CertificateHandler{
		courseService:     cs,
		blockchainService: bs,
		emailService:      es,
	}
}

// GetCertificate returns existing certificate for a course
func (h *CertificateHandler) GetCertificate(c *fiber.Ctx) error {
	courseIDStr := c.Params("courseId")
	courseID, err := strconv.ParseUint(courseIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid course ID",
		})
	}

	userID := middleware.ResolveUserID(c)

	var cert models.Certificate
	err = database.DB.Preload("User").Preload("Course").
		Where("user_id = ? AND course_id = ?", userID, courseID).
		First(&cert).Error
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "No certificate issued yet for this course",
		})
	}

	return c.JSON(fiber.Map{
		"success":     true,
		"certificate": cert,
	})
}

// MintCertificate mints an on-chain Soulbound Token and sends email notification
func (h *CertificateHandler) MintCertificate(c *fiber.Ctx) error {
	courseIDStr := c.Params("courseId")
	courseID, err := strconv.ParseUint(courseIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid course ID",
		})
	}

	userID := middleware.ResolveUserID(c)

	// Check if already minted
	var existingCert models.Certificate
	if err := database.DB.Where("user_id = ? AND course_id = ?", userID, courseID).First(&existingCert).Error; err == nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success":     true,
			"message":     "Certificate already claimed and recorded on-chain",
			"certificate": existingCert,
		})
	}

	// Verify course completion
	isCompleted, completedCount, totalCount, err := h.courseService.CheckCourseCompletion(userID, uint(courseID))
	if err != nil || !isCompleted {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success":   false,
			"message":   fmt.Sprintf("Course incomplete. You finished %d of %d lessons.", completedCount, totalCount),
			"completed": completedCount,
			"total":     totalCount,
		})
	}

	// Fetch user and course info
	var user models.User
	var course models.Course
	database.DB.First(&user, userID)
	database.DB.First(&course, courseID)

	// Calculate learning statistics
	completedCount, totalWatchSec, avgScore, _ := h.courseService.GetCourseStats(userID, uint(courseID))

	// Construct IPFS Metadata URI
	metadataURI := fmt.Sprintf("ipfs://bafkreicertificate-%d-%d-metadata.json", courseID, userID)

	// Trigger Blockchain Minting
	mintResult, err := h.blockchainService.MintSoulboundCertificate(user.WalletAddress, uint(courseID), completedCount, totalWatchSec, avgScore, metadataURI)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Failed to mint SBT certificate on blockchain",
			"error":   err.Error(),
		})
	}

	// Persist Certificate in Database
	cert := models.Certificate{
		UserID:          user.ID,
		CourseID:        course.ID,
		TokenID:         mintResult.TokenID,
		TransactionHash: mintResult.TransactionHash,
		LessonsCount:    completedCount,
		TotalWatchSec:   totalWatchSec,
		AverageScore:    avgScore,
		IssuedAt:        time.Now(),
	}
	if err := database.DB.Create(&cert).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Failed to save certificate record",
			"error":   err.Error(),
		})
	}

	// Send Email notification asynchronously
	if user.Email != "" {
		go func() {
			_ = h.emailService.SendCertificateEmail(
				user.Email,
				user.WalletAddress,
				course.Title,
				mintResult.TokenID,
				mintResult.TransactionHash,
			)
		}()
	}

	return c.JSON(fiber.Map{
		"success":          true,
		"message":          "🎉 Soulbound Certificate successfully minted!",
		"certificate":      cert,
		"transaction_hash": mintResult.TransactionHash,
		"token_id":         mintResult.TokenID,
		"is_simulated":     mintResult.IsSimulated,
		"explorer_url":     mintResult.ExplorerURL,
	})
}

// GetMyCertificates returns all certificates minted by the logged-in learner (auto-generating any missing ones for completed lessons)
func (h *CertificateHandler) GetMyCertificates(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)

	certs, _ := h.courseService.EnsureAllUserCertificates(userID, h.blockchainService)

	var activeLessonIDs []uint
	database.DB.Model(&models.Lesson{}).Pluck("id", &activeLessonIDs)
	activeSet := make(map[uint]bool, len(activeLessonIDs))
	for _, id := range activeLessonIDs {
		activeSet[id] = true
	}
	for i := range certs {
		certs[i].LessonExists = activeSet[certs[i].LessonID]
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"certificates": certs,
	})
}

// DeleteMyCertificate allows the certificate owner to delete their own certificate (e.g., old certificates from deleted lessons)
func (h *CertificateHandler) DeleteMyCertificate(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	certIDStr := c.Params("id")
	certID, err := strconv.ParseUint(certIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid certificate ID",
		})
	}

	var cert models.Certificate
	if err := database.DB.First(&cert, certID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบวุฒิบัตรที่ต้องการลบ",
		})
	}

	if cert.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "คุณไม่มีสิทธิ์ลบวุฒิบัตรของผู้อื่น",
		})
	}

	if err := database.DB.Delete(&cert).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถลบวุฒิบัตรได้",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ลบวุฒิบัตรเรียบร้อยแล้ว",
	})
}

// SendCertificateEmail sends the certificate directly to the learner's Gmail by certificate ID
func (h *CertificateHandler) SendCertificateEmail(c *fiber.Ctx) error {
	certIDStr := c.Params("id")
	certID, err := strconv.ParseUint(certIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid certificate ID",
		})
	}

	var cert models.Certificate
	if err := database.DB.Preload("Course").Preload("User").First(&cert, certID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Certificate not found",
		})
	}

	toEmail := cert.User.Email
	if toEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ผู้เรียนยังไม่มีที่อยู่อีเมลในระบบ",
		})
	}

	title := cert.LessonTitle
	if title == "" {
		title = cert.Course.Title
	}

	go func() {
		_ = h.emailService.SendCertificateEmail(
			toEmail,
			cert.User.WalletAddress,
			title,
			cert.TokenID,
			cert.TransactionHash,
		)
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("ส่งวุฒิบัตร (%s) ไปยังอีเมล %s เรียบร้อยแล้ว", title, toEmail),
	})
}

// SendLessonCertificateEmail ensures a certificate exists for (userID, lessonId) and sends it to the learner's email
func (h *CertificateHandler) SendLessonCertificateEmail(c *fiber.Ctx) error {
	lessonIDStr := c.Params("lessonId")
	lessonID, err := strconv.ParseUint(lessonIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid lesson ID",
		})
	}

	userID := middleware.ResolveUserID(c)
	cert, err := h.courseService.EnsureLessonCertificate(userID, uint(lessonID), h.blockchainService)
	if err != nil || cert == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถออกวุฒิบัตรสำหรับบทเรียนนี้ได้",
		})
	}

	toEmail := cert.User.Email
	if toEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ผู้เรียนยังไม่มีที่อยู่อีเมลในระบบ",
		})
	}

	title := cert.LessonTitle
	if title == "" {
		title = cert.Course.Title
	}

	go func() {
		_ = h.emailService.SendCertificateEmail(
			toEmail,
			cert.User.WalletAddress,
			title,
			cert.TokenID,
			cert.TransactionHash,
		)
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("ส่งวุฒิบัตรบทเรียน \"%s\" ไปยังอีเมล %s เรียบร้อยแล้ว", title, toEmail),
	})
}
