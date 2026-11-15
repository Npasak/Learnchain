package handlers

import (
	"time"

	"elearning-sbt/config"
	"elearning-sbt/services"
	"elearning-sbt/version"

	"github.com/gofiber/fiber/v2"
)

// VersionHandler exposes version metadata and self-update endpoints.
type VersionHandler struct {
	cfg            *config.Config
	updaterService *services.UpdaterService
	onShutdown     func()
}

// NewVersionHandler constructs a VersionHandler.
func NewVersionHandler(cfg *config.Config, updaterService *services.UpdaterService, onShutdown func()) *VersionHandler {
	return &VersionHandler{
		cfg:            cfg,
		updaterService: updaterService,
		onShutdown:     onShutdown,
	}
}

// GetVersion returns current build metadata and update configuration status.
func (h *VersionHandler) GetVersion(c *fiber.Ctx) error {
	info := version.GetInfo()
	return c.JSON(fiber.Map{
		"version":           info.Version,
		"commit":            info.Commit,
		"build_date":        info.BuildDate,
		"go_version":        info.GoVersion,
		"os":                info.OS,
		"arch":              info.Arch,
		"update_configured": h.cfg.UpdateRepo != "" || h.cfg.UpdateManifestURL != "",
		"update_repo":       h.cfg.UpdateRepo,
	})
}

// CheckUpdate checks the configured release source for a newer version.
func (h *VersionHandler) CheckUpdate(c *fiber.Ctx) error {
	res, err := h.updaterService.CheckForUpdate(c.Context())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "ไม่สามารถตรวจสอบเวอร์ชันล่าสุดได้",
			"details": err.Error(),
		})
	}
	return c.JSON(res)
}

type applyUpdateRequest struct {
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
}

// ApplyUpdate downloads the latest binary, backs up the DB, swaps binaries, and restarts.
func (h *VersionHandler) ApplyUpdate(c *fiber.Ctx) error {
	var body applyUpdateRequest
	_ = c.BodyParser(&body)

	downloadURL := body.DownloadURL
	sha256Hash := body.SHA256

	if downloadURL == "" {
		checkRes, err := h.updaterService.CheckForUpdate(c.Context())
		if err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"error":   "ไม่สามารถตรวจสอบไฟล์อัปเดตได้",
				"details": err.Error(),
			})
		}
		if !checkRes.Configured {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": checkRes.Message,
			})
		}
		if !checkRes.UpdateAvailable {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"status":  "up_to_date",
				"message": "คุณใช้งานเวอร์ชันล่าสุดอยู่แล้ว",
				"version": checkRes.CurrentVersion,
			})
		}
		if checkRes.DownloadURL == "" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "ไม่พบไฟล์ .exe ใน Release ล่าสุด",
			})
		}
		downloadURL = checkRes.DownloadURL
		sha256Hash = checkRes.SHA256
	}

	result, err := h.updaterService.ApplyUpdate(c.Context(), downloadURL, sha256Hash)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "อัปเดตไม่สำเร็จ",
			"details": err.Error(),
		})
	}

	// Delay shutdown slightly so the JSON response is flushed to the client.
	if h.onShutdown != nil {
		go func() {
			time.Sleep(500 * time.Millisecond)
			h.onShutdown()
		}()
	}

	return c.JSON(result)
}
