package database

import (
	"log"

	"elearning-sbt/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB initializes SQLite database connection and auto-migrates models
func InitDB(dbPath string) *gorm.DB {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	log.Println("📦 Connected to SQLite database successfully.")

	// Drop legacy unique index idx_user_course so multiple lesson certificates can be issued per user
	DB.Exec("DROP INDEX IF EXISTS idx_user_course")

	// AutoMigrate all models
	err = DB.AutoMigrate(
		&models.User{},
		&models.Course{},
		&models.Lesson{},
		&models.Quiz{},
		&models.Question{},
		&models.Option{},
		&models.UserLessonProgress{},
		&models.QuizAttempt{},
		&models.Certificate{},
		&models.LessonAssignment{},
		&models.LessonRating{},
	)
	if err != nil {
		log.Fatalf("❌ AutoMigration failed: %v", err)
	}

	log.Println("✅ Database AutoMigrate completed.")

	// Seed demo data if database is empty
	seedDemoData(DB)

	// Ensure at least one Manager exists in the system (if 0 managers exist, promote the first Creator with an email)
	var mgrCount int64
	DB.Model(&models.User{}).Where("role = ?", "manager").Count(&mgrCount)
	if mgrCount == 0 {
		var firstCreator models.User
		if err := DB.Where("role = ? AND email != ''", "creator").Order("id desc").First(&firstCreator).Error; err == nil {
			mgrUser := models.User{
				WalletAddress: firstCreator.WalletAddress + "-mgr",
				Name:          firstCreator.Name,
				Role:          "manager",
				Email:         firstCreator.Email,
				AvatarURL:     firstCreator.AvatarURL,
			}
			DB.Create(&mgrUser)
			log.Printf("👔 Bootstrapped initial Manager account from Creator: %s (%s)", firstCreator.Name, firstCreator.Email)
		}
	}

	return DB
}

// seedDemoData populates default course, lessons, and quizzes for instant out-of-the-box testing
func seedDemoData(db *gorm.DB) {
	// Auto-upgrade existing course and lessons with creator information, thumbnails, YouTube & document links
	var existingCourse models.Course
	if err := db.First(&existingCourse, 1).Error; err == nil {
		existingCourse.Title = "การพัฒนา Smart Contract และวุฒิบัตร Soulbound Token (SBT)"
		existingCourse.Summary = "หลักสูตรสำหรับนักพัฒนา: เรียนรู้ระบบฉันทามติ Web3, เขียน Smart Contract บน Solidity และออกใบรับรอง SBT บนบล็อกเชน"
		existingCourse.Description = "เรียนรู้สถาปัตยกรรม Ethereum, มาตรฐาน ERC-721 และวิธีการออกใบรับรองดิจิทัลแบบห้ามโอนย้าย (Soulbound Token) ลงบนบล็อกเชน"
		existingCourse.ThumbnailURL = "https://images.unsplash.com/photo-1639762681485-074b7f938ba0?q=80&w=800&auto=format&fit=crop"
		db.Save(&existingCourse)

		db.Model(&models.User{}).Where("id = ?", 1).Updates(map[string]interface{}{
			"name": "ผศ.ดร. ธนวัฒน์ บล็อกเชนวิทยา (Senior Blockchain Architect)",
		})

		db.Model(&models.Lesson{}).Where("id = ? AND course_id = ?", 1, 1).Updates(map[string]interface{}{
			"title":         "บทที่ 1: พื้นฐานเทคโนโลยี Web3 และกระเป๋าดิจิทัล (Wallets)",
			"summary":       "ทำความเข้าใจระบบบล็อกเชน Public-Key Cryptography และการเชื่อมต่อกระเป๋า MetaMask",
			"description":   "เจาะลึกระบบฉันทามติของบล็อกเชน วิทยาการเข้ารหัสลับแบบกุญแจสาธารณะ และการเชื่อมต่อ MetaMask กับ Smart Contract",
			"thumbnail_url": "https://images.unsplash.com/photo-1622979135225-d2ba269bc1df?q=80&w=600&auto=format&fit=crop",
			"youtube_url":   "https://www.youtube.com/watch?v=gyMwXuJrbJQ",
			"document_url":  "https://ethereum.org/en/developers/docs/",
			"duration_sec":  60, // กำหนดระยะเวลาเรียนขั้นต่ำ 1 นาที (60 วินาที) เพื่อสะดวกต่อการทดสอบ
		})

		db.Model(&models.Lesson{}).Where("id = ? AND course_id = ?", 2, 1).Updates(map[string]interface{}{
			"title":         "บทที่ 2: การพัฒนา Soulbound Token (SBT) บน Solidity และห้ามโอนย้าย",
			"summary":       "เขียน Smart Contract เพื่อระงับการโอนย้ายเหรียญ (Non-transferable) และสร้างระบบออกวุฒิบัตร",
			"description":   "เจาะลึกกลไกของ Soulbound Token การบล็อกการถ่ายโอนเหรียญบน Solidity และการบันทึกประวัติการศึกษาถาวร",
			"thumbnail_url": "https://images.unsplash.com/photo-1642104704074-907c0698cbd9?q=80&w=600&auto=format&fit=crop",
			"youtube_url":   "https://www.youtube.com/watch?v=M576WGiDBdQ",
			"document_url":  "https://eips.ethereum.org/EIPS/eip-5114",
			"duration_sec":  90, // กำหนดระยะเวลาเรียนขั้นต่ำ 1.5 นาที (90 วินาที)
		})
	}

	var userCount int64
	db.Model(&models.User{}).Count(&userCount)
	if userCount > 0 {
		return
	}

	log.Println("🌱 Seeding initial demo data for E-Learning dApp...")

	// 1. Create Creator & Demo Learner
	creator := models.User{
		WalletAddress: "0x71C83a451D14feB383021f1d1326410408544955",
		Name:          "ผศ.ดร. ธนวัฒน์ บล็อกเชนวิทยา (Senior Blockchain Architect)",
		Role:          "creator",
		Email:         "creator@learnchain.io",
	}
	db.Create(&creator)

	learner := models.User{
		WalletAddress: "0x90F79bf6EB2c4f870365E785982E1f101E93b906",
		Name:          "สมศักดิ์ ผู้เรียนรู้ Web3",
		Role:          "learner",
		Email:         "learner@example.com",
	}
	db.Create(&learner)

	// 2. Create Course
	course := models.Course{
		Title:        "การพัฒนา Smart Contract และวุฒิบัตร Soulbound Token (SBT)",
		Summary:      "หลักสูตรสำหรับนักพัฒนา: เรียนรู้ระบบฉันทามติ Web3, เขียน Smart Contract บน Solidity และออกใบรับรอง SBT บนบล็อกเชน",
		Description:  "เรียนรู้สถาปัตยกรรม Ethereum, มาตรฐาน ERC-721 และวิธีการออกใบรับรองดิจิทัลแบบห้ามโอนย้าย (Soulbound Token) ลงบนบล็อกเชน",
		ThumbnailURL: "https://images.unsplash.com/photo-1639762681485-074b7f938ba0?q=80&w=800&auto=format&fit=crop",
		CreatorID:    creator.ID,
	}
	db.Create(&course)

	// 3. Lesson 1
	lesson1 := models.Lesson{
		CourseID:     course.ID,
		Title:        "บทที่ 1: พื้นฐานเทคโนโลยี Web3 และกระเป๋าดิจิทัล (Wallets)",
		Summary:      "ทำความเข้าใจระบบบล็อกเชน Public-Key Cryptography และการเชื่อมต่อกระเป๋า MetaMask",
		Description:  "เจาะลึกระบบฉันทามติของบล็อกเชน วิทยาการเข้ารหัสลับแบบกุญแจสาธารณะ และการเชื่อมต่อ MetaMask กับ Smart Contract",
		ThumbnailURL: "https://images.unsplash.com/photo-1622979135225-d2ba269bc1df?q=80&w=600&auto=format&fit=crop",
		YoutubeURL:   "https://www.youtube.com/watch?v=gyMwXuJrbJQ",
		DocumentURL:  "https://ethereum.org/en/developers/docs/",
		DurationSec:  60, // 60 วินาที
		SortOrder:    1,
	}
	db.Create(&lesson1)

	// Pre-test for Lesson 1
	preQuiz1 := models.Quiz{
		LessonID: lesson1.ID,
		Title:    "บทที่ 1: แบบทดสอบก่อนเรียน (Pre-Assessment)",
		Type:     "lesson_pre_test",
	}
	db.Create(&preQuiz1)

	q1 := models.Question{
		QuizID:       preQuiz1.ID,
		QuestionText: "ข้อใดคือความแตกต่างสำคัญระหว่างเทคโนโลยีบล็อกเชนกับฐานข้อมูลแบบดั้งเดิม (RDBMS)?",
	}
	db.Create(&q1)
	db.Create(&models.Option{QuestionID: q1.ID, OptionText: "บล็อกเชนกระจายศูนย์และแก้ไขย้อนหลังไม่ได้ (Immutability)", IsCorrect: true})
	db.Create(&models.Option{QuestionID: q1.ID, OptionText: "บล็อกเชนต้องรันอยู่บนเซิร์ฟเวอร์ AWS เท่านั้น", IsCorrect: false})
	db.Create(&models.Option{QuestionID: q1.ID, OptionText: "บล็อกเชนไม่สามารถจัดเก็บข้อมูลธุรกรรมได้", IsCorrect: false})

	// Post-test for Lesson 1
	postQuiz1 := models.Quiz{
		LessonID: lesson1.ID,
		Title:    "บทที่ 1: แบบทดสอบวัดผลหลังเรียน (Mastery Evaluation)",
		Type:     "lesson_post_test",
	}
	db.Create(&postQuiz1)

	q2 := models.Question{
		QuizID:       postQuiz1.ID,
		QuestionText: "ในการทำธุรกรรมบน Web3 dApp อะไรคือสิ่งที่ใช้ในการเซ็นชื่อรับรองธุรกรรม?",
	}
	db.Create(&q2)
	db.Create(&models.Option{QuestionID: q2.ID, OptionText: "Private Key ของผู้ใช้ผ่านกระเป๋าเงินคริปโต (เช่น MetaMask)", IsCorrect: true})
	db.Create(&models.Option{QuestionID: q2.ID, OptionText: "รหัสผ่านของระบบ Web Hosting ส่วนกลาง", IsCorrect: false})
	db.Create(&models.Option{QuestionID: q2.ID, OptionText: "DNS Server ของผู้ให้บริการอินเทอร์เน็ต", IsCorrect: false})

	// 4. Lesson 2
	lesson2 := models.Lesson{
		CourseID:     course.ID,
		Title:        "บทที่ 2: การพัฒนา Soulbound Token (SBT) บน Solidity และห้ามโอนย้าย",
		Summary:      "เขียน Smart Contract เพื่อระงับการโอนย้ายเหรียญ (Non-transferable) และสร้างระบบออกวุฒิบัตร",
		Description:  "เจาะลึกกลไกของ Soulbound Token การบล็อกการถ่ายโอนเหรียญบน Solidity และการบันทึกประวัติการศึกษาถาวร",
		ThumbnailURL: "https://images.unsplash.com/photo-1642104704074-907c0698cbd9?q=80&w=600&auto=format&fit=crop",
		YoutubeURL:   "https://www.youtube.com/watch?v=M576WGiDBdQ",
		DocumentURL:  "https://eips.ethereum.org/EIPS/eip-5114",
		DurationSec:  90,
		SortOrder:    2,
	}
	db.Create(&lesson2)

	// Pre-test for Lesson 2
	preQuiz2 := models.Quiz{
		LessonID: lesson2.ID,
		Title:    "บทที่ 2: แบบทดสอบก่อนเรียน",
		Type:     "lesson_pre_test",
	}
	db.Create(&preQuiz2)

	q3 := models.Question{
		QuizID:       preQuiz2.ID,
		QuestionText: "คุณสมบัติสำคัญที่สุดของ Soulbound Token (SBT) คือข้อใด?",
	}
	db.Create(&q3)
	db.Create(&models.Option{QuestionID: q3.ID, OptionText: "ไม่สามารถโอนย้ายหรือซื้อขายเปลี่ยนมือไปยังกระเป๋าอื่นได้", IsCorrect: true})
	db.Create(&models.Option{QuestionID: q3.ID, OptionText: "สามารถแจกผลตอบแทน Staking APY 100% เสมอ", IsCorrect: false})
	db.Create(&models.Option{QuestionID: q3.ID, OptionText: "จะหมดอายุและเผาทำลายทิ้งภายใน 24 ชั่วโมง", IsCorrect: false})

	// Post-test for Lesson 2
	postQuiz2 := models.Quiz{
		LessonID: lesson2.ID,
		Title:    "บทที่ 2: แบบทดสอบจบหลักสูตรเพื่อรับใบประกาศนียบัตร",
		Type:     "lesson_post_test",
	}
	db.Create(&postQuiz2)

	q4 := models.Question{
		QuizID:       postQuiz2.ID,
		QuestionText: "ในสัญญาอัจฉริยะ (Smart Contract) เราป้องกันการโอนเหรียญ ERC-721 ให้เป็น Soulbound ได้อย่างไร?",
	}
	db.Create(&q4)
	db.Create(&models.Option{QuestionID: q4.ID, OptionText: "สั่ง Revert ในฟังก์ชัน transferFrom เมื่อที่อยู่ผู้ส่งและผู้รับไม่ใช่ Address 0", IsCorrect: true})
	db.Create(&models.Option{QuestionID: q4.ID, OptionText: "ลบฟังก์ชัน ownerOf ออกจากสัญญา", IsCorrect: false})
	db.Create(&models.Option{QuestionID: q4.ID, OptionText: "ตั้งค่า Gas Limit ให้เท่ากับศูนย์", IsCorrect: false})

	log.Println("✨ Demo Course, Lessons, and Quizzes seeded successfully!")
}
