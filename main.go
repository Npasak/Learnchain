package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"elearning-sbt/config"
	"elearning-sbt/database"
	"elearning-sbt/handlers"
	"elearning-sbt/middleware"
	"elearning-sbt/services"
	"elearning-sbt/version"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Embed frontend dist assets into the executable single binary
//
//go:embed dist/*
var embedDir embed.FS

func main() {
	// Clean up any leftover .old / .new files from a previous self-update
	services.CleanupOldBinary()

	banner := fmt.Sprintf(`
======================================================
  LearnChain: Decentralized E-Learning & Credentials  
     Soulbound Token (SBT) - Single-Binary dApp       
  Version: %-18s Built: %s
======================================================`, version.Version, version.BuildDate)
	fmt.Println(banner)

	// Graceful shutdown channel shared with System Tray and Auto-Updater
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	triggerQuit := func() {
		select {
		case quit <- os.Interrupt:
		default:
		}
	}

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize Database & GORM AutoMigrate
	db := database.InitDB(cfg.DBPath)

	// 3. Initialize Services
	blockchainService := services.NewBlockchainService(cfg)
	emailService := services.NewEmailService(cfg)
	courseService := services.NewCourseService(db)
	updaterService := services.NewUpdaterService(cfg)

	// 4. Initialize Handlers
	authHandler := handlers.NewAuthHandler()
	courseHandler := handlers.NewCourseHandler(courseService)
	lessonHandler := handlers.NewLessonHandler(courseService, blockchainService)
	quizHandler := handlers.NewQuizHandler(courseService, blockchainService)
	certHandler := handlers.NewCertificateHandler(courseService, blockchainService, emailService)
	managerHandler := handlers.NewManagerHandler(emailService)
	syncHandler := handlers.NewSyncHandler()
	lifecycleHandler := handlers.NewLifecycleHandler()
	godVaultHandler := handlers.NewGodVaultHandler()
	versionHandler := handlers.NewVersionHandler(cfg, updaterService, triggerQuit)
	signatoryHandler := handlers.NewSignatoryHandler()

	// 5. Initialize Fiber App
	app := fiber.New(fiber.Config{
		AppName:               "LearnChain Single Binary dApp",
		DisableStartupMessage: false,
		BodyLimit:             50 * 1024 * 1024, // 50MB for multi-machine sync snapshots
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-User-ID, X-Wallet-Address",
	}))

	// 6. Register REST API Routes
	api := app.Group("/api")
	api.Use(middleware.UserContextMiddleware())

	// Auth & Users
	api.Get("/version", versionHandler.GetVersion)
	api.Get("/update/check", versionHandler.CheckUpdate)
	api.Post("/update/apply", versionHandler.ApplyUpdate)
	api.Get("/auth/config", authHandler.GetAuthConfig)
	api.Get("/auth/check-roles", authHandler.CheckEmailRoles)
	api.Post("/auth/register", authHandler.Register)
	api.Post("/auth/google", authHandler.GoogleLogin)
	api.Post("/auth/connect", authHandler.ConnectWallet)
	api.Get("/auth/me", authHandler.GetCurrentUser)
	api.Post("/auth/profile", authHandler.UpdateProfile)
	api.Get("/users", authHandler.ListUsers)

	// Courses
	api.Get("/courses", courseHandler.ListCourses)
	api.Get("/courses/creator/:creatorId", courseHandler.GetCourseByCreator)
	api.Get("/creator/certificate-data", lessonHandler.GetCreatorCertificateData)
	api.Get("/courses/:id", courseHandler.GetCourseDetails)
	api.Put("/courses/:id", courseHandler.UpdateCourse)

	// Lessons (Guarded with Lesson Access Guard)
	lessonsGroup := api.Group("/lessons")
	lessonsGroup.Post("/cover", lessonHandler.UploadCover)
	lessonsGroup.Post("/", lessonHandler.CreateLesson)
	lessonsGroup.Put("/:lessonId", lessonHandler.UpdateLesson)
	lessonsGroup.Post("/:lessonId/toggle-hide", lessonHandler.ToggleHideLesson)
	lessonsGroup.Post("/:lessonId/schedule", lessonHandler.UpdateLessonSchedule)
	lessonsGroup.Delete("/:lessonId", lessonHandler.DeleteLesson)
	lessonsGroup.Get("/:lessonId", middleware.AccessGuardMiddleware(courseService), lessonHandler.GetLesson)
	lessonsGroup.Post("/:lessonId/progress", lessonHandler.UpdateProgress)
	lessonsGroup.Post("/:lessonId/rate", lessonHandler.RateLesson)
	lessonsGroup.Get("/:lessonId/ratings", lessonHandler.GetLessonRatings)

	// Quizzes (IsCorrect sanitization & grading)
	api.Get("/quizzes/:quizId", quizHandler.GetQuiz)
	api.Post("/quizzes/:quizId/submit", quizHandler.SubmitQuiz)

	// Certificate Signatories (Manager manages list; Creator selects per lesson)
	api.Get("/signatories", signatoryHandler.ListSignatories)
	api.Post("/signatories/upload", signatoryHandler.UploadSignatureImage)
	api.Post("/signatories", signatoryHandler.CreateSignatory)
	api.Put("/signatories/:id", signatoryHandler.UpdateSignatory)
	api.Delete("/signatories/:id", signatoryHandler.DeleteSignatory)

	// Soulbound Certificate Minting & Verification
	api.Get("/certificates/my", certHandler.GetMyCertificates)
	api.Delete("/certificates/:id", certHandler.DeleteMyCertificate)
	api.Post("/certificates/:id/email", certHandler.SendCertificateEmail)
	api.Post("/lessons/:lessonId/certificate/email", certHandler.SendLessonCertificateEmail)
	api.Get("/courses/:courseId/certificate", certHandler.GetCertificate)
	api.Post("/courses/:courseId/mint", certHandler.MintCertificate)

	// Manager
	managerGroup := api.Group("/manager")
	managerGroup.Get("/dashboard", managerHandler.GetDashboard)
	managerGroup.Post("/assign", managerHandler.AssignLearner)
	managerGroup.Delete("/assign", managerHandler.UnassignLearner)
	managerGroup.Post("/report/email", managerHandler.SendReportEmail)
	managerGroup.Post("/managers", managerHandler.AddManagerFromCreator)
	managerGroup.Delete("/managers/:id", managerHandler.RemoveManager)
	managerGroup.Get("/members", managerHandler.ListMembers)
	managerGroup.Put("/members/role", managerHandler.UpdateMemberRole)
	managerGroup.Delete("/members", managerHandler.DeleteMember)
	managerGroup.Get("/lifecycle", lifecycleHandler.GetLifecycleOverview)
	managerGroup.Put("/lifecycle/:lessonId/age", lifecycleHandler.SetBlockAgeDays)
	managerGroup.Get("/lifecycle/export", lifecycleHandler.ExportExpiredBlocks)
	managerGroup.Delete("/lifecycle/purge", lifecycleHandler.PurgeExpiredBlocks)
	managerGroup.Get("/lessons/:lessonId/learners", managerHandler.GetLessonLearnersStats)
	managerGroup.Get("/wards/:ward/learners", managerHandler.GetWardLearnersStats)

	// God Therunrock Exclusive Vault & Chain Control
	godGroup := api.Group("/god")
	godGroup.Get("/vault/status", godVaultHandler.GetVaultStatus)
	godGroup.Post("/cold-offload", godVaultHandler.ColdOffload)
	godGroup.Post("/rehydrate", godVaultHandler.StateRehydrate)
	godGroup.Delete("/oblivion-sweep", godVaultHandler.OblivionSweep)
	godGroup.Post("/snapshot", godVaultHandler.StateSnapshot)
	godGroup.Post("/distill", godVaultHandler.DataDistillation)
	godGroup.Post("/crystal-audit", godVaultHandler.CrystalAudit)
	godGroup.Post("/phoenix-reboot", godVaultHandler.PhoenixReboot)

	// Multi-Machine Sync & Network Sharing
	syncGroup := api.Group("/sync")
	syncGroup.Get("/ping", syncHandler.Ping)
	syncGroup.Get("/info", syncHandler.GetSyncInfo)
	syncGroup.Get("/qr", syncHandler.GetQRCodeSVG)
	syncGroup.Post("/config", syncHandler.UpdateSyncConfig)
	syncGroup.Post("/auto-now", syncHandler.TriggerAutoSyncNow)
	syncGroup.Get("/gas-hub", syncHandler.GasHubCompatibleEndpoint)
	syncGroup.Post("/gas-hub", syncHandler.GasHubCompatibleEndpoint)
	syncGroup.Get("/gas-script", syncHandler.GetGasScriptSource)
	syncGroup.Get("/export", syncHandler.ExportSnapshot)
	syncGroup.Post("/import", syncHandler.ImportSnapshot)
	syncGroup.Post("/peer", syncHandler.PeerSync)

	// 7. Disk Covers (uploaded/synced, read-and-close so Windows never locks files on delete) + Embedded Frontend Static Files
	_ = os.MkdirAll("./covers", 0755)
	app.Get("/covers/:filename", func(c *fiber.Ctx) error {
		fname := filepath.Base(c.Params("filename"))
		if fname != "" && fname != "." && fname != ".." {
			if data, err := os.ReadFile(filepath.Join("./covers", fname)); err == nil {
				c.Type(filepath.Ext(fname))
				return c.Send(data)
			}
		}
		return c.Next()
	})

	distFS, err := fs.Sub(embedDir, "dist")
	if err != nil {
		log.Fatalf("❌ Failed to access embedded frontend filesystem: %v", err)
	}

	app.Use("/", filesystem.New(filesystem.Config{
		Root:   http.FS(distFS),
		Index:  "index.html",
		Browse: false,
	}))

	// Fallback route for SPA navigation
	app.Use(func(c *fiber.Ctx) error {
		if !strings.HasPrefix(c.Path(), "/api") {
			file, err := distFS.Open("index.html")
			if err == nil {
				defer file.Close()
				stat, _ := file.Stat()
				c.Set("Content-Type", "text/html")
				return c.SendStream(file, int(stat.Size()))
			}
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Route Not Found",
		})
	})

	// 8. Check if port is already running (avoid black window flash on double-click)
	url := fmt.Sprintf("http://localhost:%s", cfg.Port)
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+cfg.Port, 350*time.Millisecond); err == nil {
		_ = conn.Close()
		log.Printf("⚠️ ระบบ LearnChain กำลังทำงานอยู่แล้วบนพอร์ต %s — กำลังเปิดหน้าเว็บ %s ให้ทันที...", cfg.Port, url)
		if cfg.AutoOpenBrowser {
			openBrowser(url)
		}
		time.Sleep(3 * time.Second)
		return
	}

	// 9. Auto-Open Browser Goroutine
	if cfg.AutoOpenBrowser {
		go func() {
			time.Sleep(600 * time.Millisecond)
			log.Printf("🌐 Automatically launching default browser at %s ...", url)
			openBrowser(url)
		}()
	}

	// 10. Graceful Shutdown & Server Listen
	go func() {
		log.Printf("🚀 Server running at %s", url)
		for _, ip := range handlers.GetLocalIPs() {
			log.Printf("🌐 เครื่องอื่นในวง LAN เข้าใช้งานร่วมกันได้ที่: http://%s:%s", ip, cfg.Port)
		}
		if err := app.Listen(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Server error: %v", err)
			log.Println("กด Enter เพื่อปิดหน้าต่าง...")
			_, _ = fmt.Scanln()
			os.Exit(1)
		}
	}()

	// Start Startup Notification + Windows System Tray Hover UI
	StartTrayUI(
		cfg.Port,
		syncHandler,
		func() { openBrowser(url) },
		triggerQuit,
	)

	<-quit

	log.Println("🛑 Shutting down LearnChain dApp gracefully...")
	_ = app.Shutdown()
	log.Println("👋 Goodbye!")
}

// openBrowser opens the specified URL in the system's default browser
func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // Linux and BSD systems
		cmd = "xdg-open"
		args = []string{url}
	}

	_ = exec.Command(cmd, args...).Start()
}
