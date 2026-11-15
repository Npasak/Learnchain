package services

import (
	"fmt"
	"log"
	"net/smtp"

	"elearning-sbt/config"
)

type EmailService struct {
	cfg *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{cfg: cfg}
}

// SendCertificateEmail sends an HTML notification email when a learner mints a Soulbound Certificate
func (s *EmailService) SendCertificateEmail(toEmail, recipientWallet, courseTitle, tokenID, txHash string) error {
	subject := fmt.Sprintf("🎓 Congratulations! Your Soulbound Certificate for '%s' is Ready", courseTitle)

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
  body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #f0f9ff; margin: 0; padding: 20px; }
  .card { max-width: 640px; margin: 0 auto; background: #ffffff; border-radius: 14px; overflow: hidden; border: 2px solid #bae6fd; box-shadow: 0 6px 18px rgba(2,132,199,0.12); }
  .header { background: linear-gradient(135deg, #0284c7, #0369a1); color: white; padding: 28px 24px; text-align: center; border-bottom: 3px solid #eab308; }
  .content { padding: 28px 24px; color: #334155; line-height: 1.65; }
  .badge { display: inline-block; background-color: rgba(255,255,255,0.2); color: #fef08a; padding: 5px 14px; border-radius: 9999px; font-weight: 700; font-size: 13px; margin-bottom: 10px; }
  .detail-box { background: #f0f9ff; border: 1px solid #bae6fd; border-radius: 10px; padding: 16px; margin: 18px 0; }
  .detail-row { display: flex; justify-content: space-between; margin-bottom: 8px; font-size: 14px; }
  .detail-label { color: #0369a1; font-weight: 600; }
  .detail-val { font-family: monospace; color: #0f172a; word-break: break-all; }
  .footer { text-align: center; font-size: 12.5px; color: #0369a1; font-weight: 600; padding: 16px; border-top: 1px solid #e0f2fe; background: #f8fafc; }
</style>
</head>
<body>
  <div class="card">
    <div class="header">
      <div class="badge">🩺 Soulbound Token (SBT) • วุฒิบัตรดิจิทัล</div>
      <h1 style="margin: 0; font-size: 22px;">ใบประกาศนียบัตรรับรองผลการศึกษา</h1>
      <div style="font-size: 13px; margin-top: 6px; color: #e0f2fe;">งานการพยาบาลผู้ป่วยพิเศษ 2-3 ฝ่ายการพยาบาล โรงพยาบาลศรีนครินทร์ คณะแพทยศาสตร์ มหาวิทยาลัยขอนแก่น</div>
    </div>
    <div class="content">
      <p>เรียน ผู้เข้าศึกษา,</p>
      <p>ขอแสดงความยินดี! ท่านได้ผ่านการเรียนรู้และทดสอบวัดผลตามเกณฑ์มาตรฐานในบทเรียน <strong>%s</strong> เรียบร้อยแล้ว</p>
      <p><strong>ออกโดย:</strong> งานการพยาบาลผู้ป่วยพิเศษ 2-3 ฝ่ายการพยาบาล โรงพยาบาลศรีนครินทร์ คณะแพทยศาสตร์ มหาวิทยาลัยขอนแก่น</p>
      
      <div class="detail-box">
        <div class="detail-row">
          <span class="detail-label">ผู้รับ (Wallet):</span>
          <span class="detail-val">%s</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">หมายเลขวุฒิบัตร (Token ID):</span>
          <span class="detail-val">#%s</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">On-Chain Tx Hash:</span>
          <span class="detail-val">%s</span>
        </div>
      </div>
    </div>
    <div class="footer">
      งานการพยาบาลผู้ป่วยพิเศษ 2-3 ฝ่ายการพยาบาล โรงพยาบาลศรีนครินทร์ คณะแพทยศาสตร์ มหาวิทยาลัยขอนแก่น
    </div>
  </div>
</body>
</html>
`, courseTitle, recipientWallet, tokenID, txHash)

	// If SMTP is not configured, simulate sending by logging
	if s.cfg.SMTPHost == "" || s.cfg.SMTPUser == "" {
		log.Printf("📧 [Simulated Email Sent] To: %s | Subject: %s | TokenID: %s", toEmail, subject, tokenID)
		return nil
	}

	// Send via live SMTP
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n%s\r\n%s", toEmail, subject, mime, htmlBody))

	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%s", s.cfg.SMTPHost, s.cfg.SMTPPort)

	err := smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{toEmail}, msg)
	if err != nil {
		log.Printf("⚠️ Failed to send SMTP email to %s: %v", toEmail, err)
		return err
	}

	log.Printf("📧 Live SMTP notification sent successfully to %s", toEmail)
	return nil
}

// SendReportEmail sends an HTML report (e.g. for Manager or Learner)
func (s *EmailService) SendReportEmail(toEmail, subject, reportHTML string) error {
	if s.cfg.SMTPHost == "" || s.cfg.SMTPUser == "" {
		log.Printf("📧 [Simulated Email Sent] To: %s | Subject: %s", toEmail, subject)
		return nil
	}

	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n%s\r\n%s", toEmail, subject, mime, reportHTML))

	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%s", s.cfg.SMTPHost, s.cfg.SMTPPort)

	err := smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{toEmail}, msg)
	if err != nil {
		log.Printf("⚠️ Failed to send SMTP report email to %s: %v", toEmail, err)
		return err
	}

	log.Printf("📧 Live SMTP report sent successfully to %s", toEmail)
	return nil
}
