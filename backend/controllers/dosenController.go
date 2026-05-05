package controllers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"nf-student-hub-backend/config"
	"nf-student-hub-backend/middlewares"
	"nf-student-hub-backend/utils"

	"github.com/gin-gonic/gin"
)

// =============================================
// ATTENDANCE CONTROLLERS - DIPERBAIKI
// =============================================
func CreateAttendanceSession(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var input struct {
		CourseID    string `json:"course_id" binding:"required"`
		Duration    int    `json:"duration" binding:"required,min=5,max=120"`
		PertemuanKe int    `json:"pertemuan_ke" binding:"required,min=1,max=16"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.ValidationError(c, "Invalid input: "+err.Error())
		return
	}

	// Get dosen ID
	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	// Verify dosen mengampu mata kuliah ini
	var courseName, hari string
	err = config.DB.QueryRow(`
		SELECT mk.nama, mk.hari 
		FROM mata_kuliah mk 
		WHERE mk.kode = $1 AND mk.dosen_id = $2
	`, input.CourseID, dosenID).Scan(&courseName, &hari)

	if err != nil {
		utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengampu mata kuliah ini")
		return
	}

	// Check jika sudah ada sesi aktif untuk pertemuan ini (hari ini)
	var existingSession int
	err = config.DB.QueryRow(`
		SELECT COUNT(*) 
		FROM attendance_sessions 
		WHERE dosen_id = $1 AND course_id = $2 AND pertemuan_ke = $3 
			AND status = 'active' AND expires_at > NOW()
			AND (created_at)::date = CURRENT_DATE
	`, dosenID, input.CourseID, input.PertemuanKe).Scan(&existingSession)

	if existingSession > 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "Sudah ada sesi aktif untuk pertemuan ini hari ini")
		return
	}

	// PERBAIKAN: Get student count dengan DISTINCT untuk menghindari duplikasi
	var studentCount int
	err = config.DB.QueryRow(`
		SELECT COUNT(DISTINCT mmk.mahasiswa_id) 
		FROM mahasiswa_mata_kuliah mmk 
		WHERE mmk.mata_kuliah_kode = $1
	`, input.CourseID).Scan(&studentCount)

	if err != nil {
		studentCount = 0
	}

	// Generate session data
	sessionToken := utils.GenerateRandomString(32)
	sessionCode := fmt.Sprintf("ABS-%s-P%d-%s", input.CourseID, input.PertemuanKe,
		time.Now().Format("020106150405"))
	qrToken := utils.GenerateRandomString(32)
	expiresAt := time.Now().Add(time.Duration(input.Duration) * time.Minute)

	// Insert ke attendance_sessions
	query := `
		INSERT INTO attendance_sessions 
		(dosen_id, course_id, pertemuan_ke, session_token, session_code, qr_token, expires_at, status, created_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', NOW())
		RETURNING id
	`

	var sessionID int64
	err = config.DB.QueryRow(query, dosenID, input.CourseID, input.PertemuanKe,
		sessionToken, sessionCode, qrToken, expiresAt).Scan(&sessionID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal membuat sesi: "+err.Error())
		return
	}

	// Get schedule info
	var jamMulai, jamSelesai string
	config.DB.QueryRow(`
		SELECT jam_mulai, jam_selesai 
		FROM mata_kuliah 
		WHERE kode = $1
	`, input.CourseID).Scan(&jamMulai, &jamSelesai)

	utils.SuccessResponse(c, gin.H{
		"session_id":    sessionID,
		"course_id":     input.CourseID,
		"course_name":   courseName,
		"pertemuan_ke":  input.PertemuanKe,
		"session_token": sessionToken,
		"session_code":  sessionCode,
		"qr_token":      qrToken,
		"expires_at":    expiresAt.Format("2006-01-02 15:04:05"),
		"duration":      input.Duration,
		"student_count": studentCount, // Jumlah mahasiswa yang benar (tanpa duplikasi)
		"hari":          hari,
		"jam_mulai":     jamMulai,
		"jam_selesai":   jamSelesai,
		"qr_url":        fmt.Sprintf("/api/dosen/absensi/qr/%s", sessionToken),
		"created_at":    time.Now().Format("2006-01-02 15:04:05"),
	}, "Sesi absensi berhasil dibuat untuk pertemuan ke-"+strconv.Itoa(input.PertemuanKe))
}

func RefreshSessionToken(c *gin.Context) {
	var input struct {
		SessionID int `json:"session_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid input: session_id required")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify dosen owns this session
	var dosenID int
	err := config.DB.QueryRow("SELECT dosen_id FROM attendance_sessions WHERE id = $1", input.SessionID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// Verify dosen
	var actualDosenID int
	err = config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $2", userID).Scan(&actualDosenID)
	if err != nil || dosenID != actualDosenID {
		utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak memiliki akses ke sesi ini")
		return
	}

	// Check if session is still active
	var expiresAt time.Time
	var status string
	err = config.DB.QueryRow(`
		SELECT expires_at, status 
		FROM attendance_sessions 
		WHERE id = $1 AND status = 'active'
	`, input.SessionID).Scan(&expiresAt, &status)

	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	if status != "active" || expiresAt.Before(time.Now()) {
		utils.ErrorResponse(c, http.StatusBadRequest, "Sesi sudah tidak aktif")
		return
	}

	// Generate new token (simple token untuk QR)
	newToken := fmt.Sprintf("%s-%d", utils.GenerateRandomString(20), time.Now().Unix())

	// Update token
	_, err = config.DB.Exec(`
		UPDATE attendance_sessions 
		SET session_token = $1, updated_at = NOW() 
		WHERE id = $2
	`, newToken, input.SessionID)

	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal refresh token: "+err.Error())
		return
	}

	// Get updated session info
	var sessionToken, sessionCode, courseID string
	var pertemuanKe int
	err = config.DB.QueryRow(`
		SELECT session_token, session_code, course_id, pertemuan_ke 
		FROM attendance_sessions 
		WHERE id = $1
	`, input.SessionID).Scan(&sessionToken, &sessionCode, &courseID, &pertemuanKe)

	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil info sesi")
		return
	}

	// Get course info
	var courseName string
	config.DB.QueryRow("SELECT nama FROM mata_kuliah WHERE kode = $1", courseID).Scan(&courseName)

	utils.SuccessResponse(c, gin.H{
		"session_id":        input.SessionID,
		"session_token":     sessionToken,
		"session_code":      sessionCode,
		"course_id":         courseID,
		"course_name":       courseName,
		"pertemuan_ke":      pertemuanKe,
		"expires_at":        expiresAt.Format("2006-01-02 15:04:05"),
		"updated_at":        time.Now().Format("2006-01-02 15:04:05"),
		"time_left_seconds": int(time.Until(expiresAt).Seconds()),
		"qr_url":            fmt.Sprintf("/api/dosen/absensi/qr/%s", sessionToken),
	}, "Token berhasil di-refresh")
}

// GetAttendanceSessionDetail - Get detail sesi dengan daftar mahasiswa dan filter pertemuan
func GetAttendanceSessionDetail(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		utils.ValidationError(c, "Session ID required")
		return
	}

	id, err := strconv.Atoi(sessionID)
	if err != nil {
		utils.ValidationError(c, "Invalid session ID")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify dosen owns this session
	var dosenID int
	err = config.DB.QueryRow("SELECT dosen_id FROM attendance_sessions WHERE id = $1", id).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// Verify dosen
	var actualDosenID int
	err = config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&actualDosenID)
	if err != nil || dosenID != actualDosenID {
		utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak memiliki akses ke sesi ini")
		return
	}

	// Get session info dengan pertemuan
	var courseID, courseName string
	var expiresAt, createdAt time.Time
	var sessionToken, sessionCode, qrToken string
	var pertemuanKe int
	err = config.DB.QueryRow(`
		SELECT asess.course_id, mk.nama, asess.expires_at, asess.created_at,
		       asess.session_token, asess.session_code, asess.qr_token, asess.pertemuan_ke
		FROM attendance_sessions asess
		JOIN mata_kuliah mk ON asess.course_id = mk.kode
		WHERE asess.id = $1
	`, id).Scan(&courseID, &courseName, &expiresAt, &createdAt, &sessionToken, &sessionCode, &qrToken, &pertemuanKe)

	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// PERBAIKAN: Query untuk menghindari duplikasi mahasiswa
	rows, err := config.DB.Query(`
		SELECT 
			m.id, 
			m.nim, 
			m.name,
			COALESCE(a.status, 'belum') as attendance_status,
			COALESCE(TO_CHAR(a.created_at, 'HH24:MI'), '') as attendance_time,
			a.created_at as attendance_created
		FROM mahasiswa m
		LEFT JOIN attendance a ON m.id = a.student_id 
			AND a.session_id = $1
			AND (a.created_at)::date = CURRENT_DATE
		WHERE m.id IN (
			SELECT DISTINCT mmk.mahasiswa_id 
			FROM mahasiswa_mata_kuliah mmk 
			WHERE mmk.mata_kuliah_kode = $2
		)
		ORDER BY m.name
	`, id, courseID)

	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil data mahasiswa: "+err.Error())
		return
	}
	defer rows.Close()

	var students []gin.H
	var totalStudents, hadirCount, izinCount, sakitCount, alpaCount int

	for rows.Next() {
		var studentID int
		var nim, name, attendanceStatus, attendanceTime string
		var attendanceCreated sql.NullTime

		err := rows.Scan(&studentID, &nim, &name, &attendanceStatus, &attendanceTime, &attendanceCreated)
		if err != nil {
			continue
		}

		totalStudents++
		switch attendanceStatus {
		case "hadir":
			hadirCount++
		case "izin":
			izinCount++
		case "sakit":
			sakitCount++
		case "alpa":
			alpaCount++
		}

		students = append(students, gin.H{
			"id":                 studentID,
			"nim":                nim,
			"name":               name,
			"attendance_status":  attendanceStatus,
			"attendance_time":    attendanceTime,
			"attendance_created": attendanceCreated,
			"status_color":       getStatusColor(attendanceStatus),
			"status_label":       getStatusLabel(attendanceStatus),
		})
	}

	// Get course schedule
	var hari, jamMulai, jamSelesai string
	config.DB.QueryRow(`
		SELECT hari, jam_mulai, jam_selesai 
		FROM mata_kuliah 
		WHERE kode = $1
	`, courseID).Scan(&hari, &jamMulai, &jamSelesai)

	// Calculate percentages
	var hadirPercent, izinPercent, sakitPercent, alpaPercent float64
	if totalStudents > 0 {
		hadirPercent = float64(hadirCount) / float64(totalStudents) * 100
		izinPercent = float64(izinCount) / float64(totalStudents) * 100
		sakitPercent = float64(sakitCount) / float64(totalStudents) * 100
		alpaPercent = float64(alpaCount) / float64(totalStudents) * 100
	}

	// Get pertemuan summary
	pertemuanRows, _ := config.DB.Query(`
		SELECT 
			pertemuan_ke,
			COUNT(*) as total_sessions,
			SUM(CASE WHEN status = 'active' AND expires_at > NOW() THEN 1 ELSE 0 END) as active_sessions
		FROM attendance_sessions 
		WHERE course_id = $1 AND dosen_id = $2
		GROUP BY pertemuan_ke
		ORDER BY pertemuan_ke
	`, courseID, dosenID)

	var pertemuanSummary []gin.H
	if pertemuanRows != nil {
		defer pertemuanRows.Close()
		for pertemuanRows.Next() {
			var pertemuan, totalSessions, activeSessions int
			pertemuanRows.Scan(&pertemuan, &totalSessions, &activeSessions)
			pertemuanSummary = append(pertemuanSummary, gin.H{
				"pertemuan_ke":    pertemuan,
				"total_sessions":  totalSessions,
				"active_sessions": activeSessions,
			})
		}
	}

	utils.SuccessResponse(c, gin.H{
		"session_id":        id,
		"course_id":         courseID,
		"course_name":       courseName,
		"pertemuan_ke":      pertemuanKe,
		"session_token":     sessionToken,
		"session_code":      sessionCode,
		"qr_token":          qrToken,
		"expires_at":        expiresAt.Format("2006-01-02 15:04:05"),
		"created_at":        createdAt.Format("2006-01-02 15:04:05"),
		"hari":              hari,
		"jam_mulai":         jamMulai,
		"jam_selesai":       jamSelesai,
		"total_students":    totalStudents,
		"hadir_count":       hadirCount,
		"izin_count":        izinCount,
		"sakit_count":       sakitCount,
		"alpa_count":        alpaCount,
		"hadir_percent":     hadirPercent,
		"izin_percent":      izinPercent,
		"sakit_percent":     sakitPercent,
		"alpa_percent":      alpaPercent,
		"students":          students,
		"time_left":         int(time.Until(expiresAt).Seconds()),
		"is_active":         expiresAt.After(time.Now()),
		"pertemuan_summary": pertemuanSummary,
	}, "Detail sesi berhasil diambil")
}

// UpdateAttendanceStatus - Update status kehadiran manual oleh dosen - DIPERBAIKI
func UpdateAttendanceStatus(c *gin.Context) {
	var input struct {
		SessionID int    `json:"session_id" binding:"required"`
		StudentID int    `json:"student_id" binding:"required"`
		Status    string `json:"status" binding:"required,oneof=hadir izin sakit alpa"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.ValidationError(c, "Invalid input: "+err.Error())
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify dosen owns this session
	var dosenID int
	err := config.DB.QueryRow("SELECT dosen_id FROM attendance_sessions WHERE id = $1", input.SessionID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// Verify dosen
	var actualDosenID int
	err = config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $2", userID).Scan(&actualDosenID)
	if err != nil || dosenID != actualDosenID {
		utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak memiliki akses ke sesi ini")
		return
	}

	// Get session info untuk pertemuan
	var courseID string
	var pertemuanKe int
	err = config.DB.QueryRow(`
		SELECT course_id, pertemuan_ke 
		FROM attendance_sessions 
		WHERE id = $1
	`, input.SessionID).Scan(&courseID, &pertemuanKe)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// Check if student is enrolled in this course
	var enrolled bool
	err = config.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM mahasiswa_mata_kuliah 
			WHERE mahasiswa_id = $1 AND mata_kuliah_kode = $2
		)
	`, input.StudentID, courseID).Scan(&enrolled)

	if err != nil || !enrolled {
		utils.ErrorResponse(c, http.StatusForbidden, "Mahasiswa tidak terdaftar di mata kuliah ini")
		return
	}

	// Check if attendance already exists untuk pertemuan ini (hari ini)
	var attendanceID int
	err = config.DB.QueryRow(`
		SELECT a.id 
		FROM attendance a
		WHERE a.student_id = $1 
			AND a.session_id = $2
			AND (a.created_at)::date = CURRENT_DATE
		LIMIT 1
	`, input.StudentID, input.SessionID).Scan(&attendanceID)

	var result sql.Result
	if err == nil && attendanceID > 0 {
		// Update existing attendance
		result, err = config.DB.Exec(`
			UPDATE attendance 
			SET status = $1, pertemuan_ke = $2, updated_at = NOW() 
			WHERE id = $3
		`, input.Status, pertemuanKe, attendanceID)
	} else {
		// Insert new attendance
		var studentCode string
		config.DB.QueryRow("SELECT nim FROM mahasiswa WHERE id = $1", input.StudentID).Scan(&studentCode)

		result, err = config.DB.Exec(`
			INSERT INTO attendance (student_id, session_id, student_code, status, pertemuan_ke, created_at)
			VALUES ($1, $2, $3, $4, $5, NOW())
		`, input.StudentID, input.SessionID, studentCode, input.Status, pertemuanKe)
	}

	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal update status: "+err.Error())
		return
	}

	// Update attendance_summary
	_, err = config.DB.Exec(`
		INSERT INTO attendance_summary 
		(student_id, nim, student_name, session_id, course_id, course_name, status, 
		 attendance_date, attendance_time, dosen_name, hari, jam_mulai, jam_selesai)
		SELECT 
			m.id, m.nim, m.name, $1, mk.kode, mk.nama, $2,
			CURRENT_DATE, NOW(), d.name, mk.hari, mk.jam_mulai, mk.jam_selesai
		FROM mahasiswa m
		JOIN mata_kuliah mk ON mk.kode = $3
		JOIN dosen d ON mk.dosen_id = d.id
		WHERE m.id = $4
		ON CONFLICT (student_id, session_id) DO UPDATE SET
			status = $5,
			attendance_time = NOW()
	`, input.SessionID, input.Status, courseID, input.StudentID, input.Status)

	rowsAffected, _ := result.RowsAffected()

	// Get student info for response
	var studentName, nim string
	config.DB.QueryRow("SELECT name, nim FROM mahasiswa WHERE id = $1", input.StudentID).Scan(&studentName, &nim)

	utils.SuccessResponse(c, gin.H{
		"student_id":    input.StudentID,
		"student_name":  studentName,
		"nim":           nim,
		"pertemuan_ke":  pertemuanKe,
		"status":        input.Status,
		"status_label":  getStatusLabel(input.Status),
		"status_color":  getStatusColor(input.Status),
		"updated_at":    time.Now().Format("2006-01-02 15:04:05"),
		"rows_affected": rowsAffected,
	}, "Status kehadiran berhasil diupdate: "+getStatusLabel(input.Status)+
		" untuk pertemuan ke-"+strconv.Itoa(pertemuanKe))
}

// GetQRCode - Generate QR Code data
func GetQRCode(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token diperlukan"})
		return
	}

	var sessionID, pertemuanKe int
	var expiresAt time.Time
	var courseID string
	err := config.DB.QueryRow(`
		SELECT id, expires_at, course_id, pertemuan_ke 
		FROM attendance_sessions 
		WHERE session_token = $1 AND status = 'active'
	`, token).Scan(&sessionID, &expiresAt, &courseID, &pertemuanKe)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "QR Code tidak valid atau sudah expired",
			"valid":   false,
			"message": "Sesi tidak ditemukan atau sudah tidak aktif",
		})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "QR Code sudah kadaluarsa",
			"valid":   false,
			"expired": true,
			"message": "Sesi sudah berakhir",
		})
		return
	}

	// Get course info
	var courseName, hari, jamMulai, jamSelesai string
	config.DB.QueryRow(`
		SELECT nama, hari, jam_mulai, jam_selesai 
		FROM mata_kuliah 
		WHERE kode = $1
	`, courseID).Scan(&courseName, &hari, &jamMulai, &jamSelesai)

	timeLeft := int(time.Until(expiresAt).Seconds())
	if timeLeft < 0 {
		timeLeft = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":         true,
		"session_token": token,
		"session_id":    sessionID,
		"course_id":     courseID,
		"course_name":   courseName,
		"pertemuan_ke":  pertemuanKe,
		"hari":          hari,
		"jam_mulai":     jamMulai,
		"jam_selesai":   jamSelesai,
		"expires_at":    expiresAt.Format("2006-01-02 15:04:05"),
		"time_left":     timeLeft,
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"qr_data":       fmt.Sprintf("%s|%s|%d|%d", token, courseID, pertemuanKe, time.Now().Unix()),
	})
}

// CloseAttendanceSession - Tutup sesi absensi (FIXED)
func CloseAttendanceSession(c *gin.Context) {
	var input struct {
		SessionID int `json:"session_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.ValidationError(c, "Invalid input")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify dosen owns this session
	var dosenID int
	err := config.DB.QueryRow("SELECT dosen_id FROM attendance_sessions WHERE id = $1", input.SessionID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// Verify dosen
	var actualDosenID int
	err = config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $2", userID).Scan(&actualDosenID)
	if err != nil || dosenID != actualDosenID {
		utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak memiliki akses ke sesi ini")
		return
	}

	// Get session info before closing
	var courseID, courseName string
	var pertemuanKe int
	var attendanceCount int
	err = config.DB.QueryRow(`
		SELECT asess.course_id, mk.nama, asess.pertemuan_ke,
		       COUNT(DISTINCT a.id) as attendance_count
		FROM attendance_sessions asess
		JOIN mata_kuliah mk ON asess.course_id = mk.kode
		LEFT JOIN attendance a ON asess.id = a.session_id
		WHERE asess.id = $1
		GROUP BY asess.id
	`, input.SessionID).Scan(&courseID, &courseName, &pertemuanKe, &attendanceCount)

	// Close session
	_, err = config.DB.Exec(`
		UPDATE attendance_sessions 
		SET status = 'closed'
		WHERE id = $1
	`, input.SessionID)

	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal menutup sesi: "+err.Error())
		return
	}

	utils.SuccessResponse(c, gin.H{
		"session_id":       input.SessionID,
		"course_id":        courseID,
		"course_name":      courseName,
		"pertemuan_ke":     pertemuanKe,
		"attendance_count": attendanceCount,
		"closed_at":        time.Now().Format("2006-01-02 15:04:05"),
	}, "Sesi berhasil ditutup untuk pertemuan ke-"+strconv.Itoa(pertemuanKe))
}

// GetActiveSessions - Get semua sesi aktif dosen dengan filter pertemuan - DIPERBAIKI
func GetActiveSessions(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get query parameter untuk filter pertemuan
	pertemuanFilter := c.Query("pertemuan_ke")
	courseFilter := c.Query("course_id")

	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	// Build query dengan filter - DIPERBAIKI untuk menghindari duplikasi
	query := `
		SELECT 
			asess.id, 
			asess.course_id,
			mk.nama as course_name,
			mk.hari,
			mk.jam_mulai,
			mk.jam_selesai,
			asess.session_token,
			asess.session_code,
			asess.expires_at,
			asess.created_at,
			asess.status,
			asess.pertemuan_ke,
			COUNT(DISTINCT a.id) as attendance_count,
			COUNT(DISTINCT CASE WHEN a.status = 'hadir' THEN a.id END) as hadir_count,
			COUNT(DISTINCT CASE WHEN a.status = 'izin' THEN a.id END) as izin_count,
			COUNT(DISTINCT CASE WHEN a.status = 'sakit' THEN a.id END) as sakit_count,
			COUNT(DISTINCT CASE WHEN a.status = 'alpa' THEN a.id END) as alpa_count,
			(
				SELECT COUNT(DISTINCT mmk.mahasiswa_id) 
				FROM mahasiswa_mata_kuliah mmk 
				WHERE mmk.mata_kuliah_kode = asess.course_id
			) as total_students
		FROM attendance_sessions asess
		JOIN mata_kuliah mk ON asess.course_id = mk.kode
		LEFT JOIN attendance a ON asess.id = a.session_id
		WHERE asess.dosen_id = $1 
			AND asess.status = 'active' 
			AND asess.expires_at > NOW()
	`

	args := []interface{}{dosenID}

	if pertemuanFilter != "" {
		query += fmt.Sprintf(" AND asess.pertemuan_ke = $%d", len(args)+1)
		args = append(args, pertemuanFilter)
	}

	if courseFilter != "" {
		query += fmt.Sprintf(" AND asess.course_id = $%d", len(args)+1)
		args = append(args, courseFilter)
	}

	query += `
		GROUP BY asess.id
		ORDER BY asess.pertemuan_ke, asess.created_at DESC
	`

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil sesi aktif: "+err.Error())
		return
	}
	defer rows.Close()

	var sessions []gin.H
	for rows.Next() {
		var id, pertemuanKe, attendanceCount, hadirCount, izinCount, sakitCount, alpaCount, totalStudents int
		var courseID, courseName, hari, jamMulai, jamSelesai, sessionToken, sessionCode, status string
		var expiresAt, createdAt time.Time

		err := rows.Scan(&id, &courseID, &courseName, &hari, &jamMulai, &jamSelesai,
			&sessionToken, &sessionCode, &expiresAt, &createdAt, &status, &pertemuanKe,
			&attendanceCount, &hadirCount, &izinCount, &sakitCount, &alpaCount, &totalStudents)
		if err != nil {
			continue
		}

		// Calculate time left
		timeLeft := int(time.Until(expiresAt).Minutes())
		if timeLeft < 0 {
			timeLeft = 0
		}

		// Calculate percentages
		var hadirPercent, belumPercent float64
		if totalStudents > 0 {
			hadirPercent = float64(hadirCount) / float64(totalStudents) * 100
			belumPercent = float64(totalStudents-attendanceCount) / float64(totalStudents) * 100
		}

		sessions = append(sessions, gin.H{
			"id":                id,
			"course_id":         courseID,
			"course_name":       courseName,
			"hari":              hari,
			"jam_mulai":         jamMulai,
			"jam_selesai":       jamSelesai,
			"session_token":     sessionToken,
			"session_code":      sessionCode,
			"expires_at":        expiresAt.Format("2006-01-02 15:04:05"),
			"created_at":        createdAt.Format("2006-01-02 15:04:05"),
			"status":            status,
			"pertemuan_ke":      pertemuanKe,
			"attendance_count":  attendanceCount,
			"hadir_count":       hadirCount,
			"izin_count":        izinCount,
			"sakit_count":       sakitCount,
			"alpa_count":        alpaCount,
			"total_students":    totalStudents,
			"hadir_percent":     hadirPercent,
			"belum_percent":     belumPercent,
			"time_left_minutes": timeLeft,
			"is_expired":        expiresAt.Before(time.Now()),
		})
	}

	utils.SuccessResponse(c, gin.H{
		"sessions":     sessions,
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"total_active": len(sessions),
	}, "Sesi aktif berhasil diambil")
}

// GetAttendanceByPertemuan - Get attendance summary per pertemuan
func GetAttendanceByPertemuan(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := c.Query("course_id")
	pertemuanKe := c.Query("pertemuan_ke")

	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	// PERBAIKAN: Query untuk menghindari duplikasi data
	query := `
		SELECT 
			m.id as student_id,
			m.nim,
			m.name as student_name,
			COALESCE(a.status, 'belum') as status,
			COALESCE(a.created_at, NULL) as attendance_time,
			asess.pertemuan_ke,
			asess.created_at as session_time
		FROM mahasiswa m
		LEFT JOIN attendance_sessions asess ON asess.course_id = $1 AND asess.dosen_id = $2
		LEFT JOIN attendance a ON m.id = a.student_id AND a.session_id = asess.id
		WHERE m.id IN (
			SELECT DISTINCT mmk.mahasiswa_id 
			FROM mahasiswa_mata_kuliah mmk 
			WHERE mmk.mata_kuliah_kode = $3
		)
	`

	args := []interface{}{courseID, dosenID, courseID}

	if pertemuanKe != "" {
		query += fmt.Sprintf(" AND asess.pertemuan_ke = $%d", len(args)+1)
		args = append(args, pertemuanKe)
	}

	query += " ORDER BY m.name, asess.pertemuan_ke"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil data absensi: "+err.Error())
		return
	}
	defer rows.Close()

	var attendanceData []gin.H
	for rows.Next() {
		var studentID, pertemuan int
		var nim, studentName, status string
		var attendanceTime, sessionTime sql.NullTime

		err := rows.Scan(&studentID, &nim, &studentName, &status, &attendanceTime, &pertemuan, &sessionTime)
		if err != nil {
			continue
		}

		attendanceData = append(attendanceData, gin.H{
			"student_id":      studentID,
			"nim":             nim,
			"student_name":    studentName,
			"status":          status,
			"pertemuan_ke":    pertemuan,
			"attendance_time": attendanceTime,
			"session_time":    sessionTime,
		})
	}

	// Get course info
	var courseName string
	config.DB.QueryRow("SELECT nama FROM mata_kuliah WHERE kode = $2", courseID).Scan(&courseName)

	utils.SuccessResponse(c, gin.H{
		"course_id":     courseID,
		"course_name":   courseName,
		"pertemuan_ke":  pertemuanKe,
		"attendance":    attendanceData,
		"total_records": len(attendanceData),
	}, "Data absensi per pertemuan berhasil diambil")
}

// GetRiwayatPertemuanDosen - Get riwayat pertemuan dosen
func GetRiwayatPertemuanDosen(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	courseID := c.Query("course_id")
	pertemuanKe := c.Query("pertemuan_ke")

	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $3", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	query := `
		SELECT 
			asess.id,
			asess.course_id,
			mk.nama as course_name,
			asess.pertemuan_ke,
			asess.created_at,
			asess.expires_at,
			asess.status,
			COUNT(DISTINCT a.id) as attendance_count
		FROM attendance_sessions asess
		JOIN mata_kuliah mk ON asess.course_id = mk.kode
		LEFT JOIN attendance a ON asess.id = a.session_id
		WHERE asess.dosen_id = $1
	`

	args := []interface{}{dosenID}

	if courseID != "" {
		query += fmt.Sprintf(" AND asess.course_id = $%d", len(args)+1)
		args = append(args, courseID)
	}

	if pertemuanKe != "" {
		query += fmt.Sprintf(" AND asess.pertemuan_ke = $%d", len(args)+1)
		args = append(args, pertemuanKe)
	}

	query += `
		GROUP BY asess.id
		ORDER BY asess.created_at DESC
		LIMIT 50
	`

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil riwayat pertemuan: "+err.Error())
		return
	}
	defer rows.Close()

	var history []gin.H
	for rows.Next() {
		var id, pertemuan, attendanceCount int
		var courseID, courseName, status string
		var createdAt, expiresAt time.Time

		err := rows.Scan(&id, &courseID, &courseName, &pertemuan, &createdAt, &expiresAt, &status, &attendanceCount)
		if err != nil {
			continue
		}

		history = append(history, gin.H{
			"id":               id,
			"course_id":        courseID,
			"course_name":      courseName,
			"pertemuan_ke":     pertemuan,
			"created_at":       createdAt.Format("2006-01-02 15:04:05"),
			"expires_at":       expiresAt.Format("2006-01-02 15:04:05"),
			"status":           status,
			"attendance_count": attendanceCount,
			"duration_minutes": int(expiresAt.Sub(createdAt).Minutes()),
		})
	}

	utils.SuccessResponse(c, gin.H{
		"history": history,
		"total":   len(history),
	}, "Riwayat pertemuan berhasil diambil")
}

// GetRealtimeAttendance - Get realtime attendance data untuk session tertentu
func GetRealtimeAttendance(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		utils.ValidationError(c, "Session ID required")
		return
	}

	id, err := strconv.Atoi(sessionID)
	if err != nil {
		utils.ValidationError(c, "Invalid session ID")
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Verify dosen owns this session
	var dosenID int
	err = config.DB.QueryRow("SELECT dosen_id FROM attendance_sessions WHERE id = $1", id).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// Verify dosen
	var actualDosenID int
	err = config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $2", userID).Scan(&actualDosenID)
	if err != nil || dosenID != actualDosenID {
		utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak memiliki akses ke sesi ini")
		return
	}

	// Get realtime attendance data
	rows, err := config.DB.Query(`
		SELECT 
			m.nim,
			m.name,
			COALESCE(a.status, 'belum') as status,
			COALESCE(TO_CHAR(a.created_at, 'HH24:MI:SS'), '') as waktu_absen
		FROM mahasiswa m
		WHERE m.id IN (
			SELECT DISTINCT mmk.mahasiswa_id 
			FROM mahasiswa_mata_kuliah mmk 
			JOIN attendance_sessions asess ON mmk.mata_kuliah_kode = asess.course_id
			WHERE asess.id = $1
		)
		LEFT JOIN attendance a ON m.id = a.student_id 
			AND a.session_id = $2
			AND (a.created_at)::date = CURRENT_DATE
		ORDER BY a.created_at DESC
	`, id, id)

	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil data realtime: "+err.Error())
		return
	}
	defer rows.Close()

	var students []gin.H
	var hadirCount, izinCount, sakitCount, alpaCount, belumCount int

	for rows.Next() {
		var nim, name, status, waktuAbsen string
		err := rows.Scan(&nim, &name, &status, &waktuAbsen)
		if err != nil {
			continue
		}

		switch status {
		case "hadir":
			hadirCount++
		case "izin":
			izinCount++
		case "sakit":
			sakitCount++
		case "alpa":
			alpaCount++
		default:
			belumCount++
		}

		students = append(students, gin.H{
			"nim":          nim,
			"name":         name,
			"status":       status,
			"waktu_absen":  waktuAbsen,
			"status_color": getStatusColor(status),
			"status_label": getStatusLabel(status),
		})
	}

	// Get session info
	var courseID, courseName string
	var expiresAt time.Time
	config.DB.QueryRow(`
		SELECT asess.course_id, mk.nama, asess.expires_at
		FROM attendance_sessions asess
		JOIN mata_kuliah mk ON asess.course_id = mk.kode
		WHERE asess.id = $1
	`, id).Scan(&courseID, &courseName, &expiresAt)

	totalStudents := hadirCount + izinCount + sakitCount + alpaCount + belumCount
	var hadirPercent float64
	if totalStudents > 0 {
		hadirPercent = float64(hadirCount) / float64(totalStudents) * 100
	}

	utils.SuccessResponse(c, gin.H{
		"session_id":  id,
		"course_id":   courseID,
		"course_name": courseName,
		"expires_at":  expiresAt.Format("2006-01-02 15:04:05"),
		"time_left":   int(time.Until(expiresAt).Seconds()),
		"students":    students,
		"summary": gin.H{
			"total_students": totalStudents,
			"hadir":          hadirCount,
			"izin":           izinCount,
			"sakit":          sakitCount,
			"alpa":           alpaCount,
			"belum":          belumCount,
			"hadir_percent":  hadirPercent,
		},
		"last_update": time.Now().Format("2006-01-02 15:04:05"),
	}, "Data realtime absensi berhasil diambil")
}

// =============================================
// HELPER FUNCTIONS
// =============================================

func getStatusColor(status string) string {
	switch status {
	case "hadir":
		return "bg-green-100 text-green-800"
	case "izin":
		return "bg-yellow-100 text-yellow-800"
	case "sakit":
		return "bg-blue-100 text-blue-800"
	case "alpa":
		return "bg-red-100 text-red-800"
	default:
		return "bg-gray-100 text-gray-800"
	}
}

func getStatusLabel(status string) string {
	switch status {
	case "hadir":
		return "Hadir"
	case "izin":
		return "Izin"
	case "sakit":
		return "Sakit"
	case "alpa":
		return "Alpa"
	default:
		return "Belum Absen"
	}
}

// isSuperDosen - Delegasi ke helper terpusat di middlewares.
// Dipertahankan sebagai wrapper agar semua controller yang sudah memanggil fungsi ini
// tidak perlu diubah.
func isSuperDosen(userID interface{}) bool {
	return middlewares.IsSuperDosen(userID)
}

// =============================================
// EXISTING DOSEN FUNCTIONS (DIPERTAHANKAN - tidak diubah)
// =============================================

/*
// GetDosenProfile - Get profile dosen
func GetDosenProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var profile struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		NIDN      string `json:"nidn"`
		Email     string `json:"email"`
		Phone     string `json:"phone"`
		Avatar    string `json:"avatar"`
		CreatedAt string `json:"created_at"`
	}

	query := `
		SELECT COALESCE(d.id, 0), COALESCE(d.name, 'Dosen'), COALESCE(d.nidn, ''), 
		       u.email, COALESCE(d.phone, ''), COALESCE(d.avatar, ''), u.created_at
		FROM users u
		LEFT JOIN dosen d ON u.id = d.user_id
		WHERE u.id = $1
	`

	err := config.DB.QueryRow(query, userID).Scan(
		&profile.ID, &profile.Name, &profile.NIDN, &profile.Email,
		&profile.Phone, &profile.Avatar, &profile.CreatedAt,
	)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Profile not found")
		return
	}

	// Get teaching statistics
	var courseCount, studentCount, sessionCount int
	config.DB.QueryRow(`
		SELECT 
			COUNT(DISTINCT mk.kode) as course_count,
			COUNT(DISTINCT mmk.mahasiswa_id) as student_count,
			COUNT(DISTINCT asess.id) as session_count
		FROM dosen d
		LEFT JOIN mata_kuliah mk ON d.id = mk.dosen_id
		LEFT JOIN mahasiswa_mata_kuliah mmk ON mk.kode = mmk.mata_kuliah_kode
		LEFT JOIN attendance_sessions asess ON d.id = asess.dosen_id
		WHERE d.id = $1
	`, profile.ID).Scan(&courseCount, &studentCount, &sessionCount)

	utils.SuccessResponse(c, gin.H{
		"profile": profile,
		"statistics": gin.H{
			"course_count":  courseCount,
			"student_count": studentCount,
			"session_count": sessionCount,
		},
	}, "Profile retrieved successfully")
}
*/

// GetDosenStats - Get statistics for dosen dashboard
func GetDosenStats(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	// Initialize variables
	var stats = gin.H{}
	superDosen := isSuperDosen(userID)

	// 1. Total courses taught
	var courseCount int
	if superDosen {
		config.DB.QueryRow(`SELECT COUNT(*) FROM mata_kuliah WHERE semester = 4`).Scan(&courseCount)
	} else {
		config.DB.QueryRow(`SELECT COUNT(*) FROM mata_kuliah WHERE dosen_id = $1`, dosenID).Scan(&courseCount)
	}
	stats["total_courses"] = courseCount

	// 2. Total students
	var studentCount int
	if superDosen {
		config.DB.QueryRow(`
			SELECT COUNT(DISTINCT mmk.mahasiswa_id) 
			FROM mata_kuliah mk
			JOIN mahasiswa_mata_kuliah mmk ON mk.kode = mmk.mata_kuliah_kode
			WHERE mk.semester = 4
		`).Scan(&studentCount)
	} else {
		config.DB.QueryRow(`
			SELECT COUNT(DISTINCT mmk.mahasiswa_id) 
			FROM mata_kuliah mk
			JOIN mahasiswa_mata_kuliah mmk ON mk.kode = mmk.mata_kuliah_kode
			WHERE mk.dosen_id = $1
		`, dosenID).Scan(&studentCount)
	}
	stats["total_students"] = studentCount

	// 3. Today's sessions
	var todaySessions int
	if superDosen {
		config.DB.QueryRow(`
			SELECT COUNT(asess.id) 
			FROM attendance_sessions asess
			JOIN mata_kuliah mk ON asess.course_id = mk.kode
			WHERE mk.semester = 4 AND (asess.created_at)::date = CURRENT_DATE
		`).Scan(&todaySessions)
	} else {
		config.DB.QueryRow(`
			SELECT COUNT(*) 
			FROM attendance_sessions 
			WHERE dosen_id = $1 AND (created_at)::date = CURRENT_DATE
		`, dosenID).Scan(&todaySessions)
	}
	stats["today_sessions"] = todaySessions

	// 4. Active sessions
	var activeSessions int
	if superDosen {
		config.DB.QueryRow(`
			SELECT COUNT(asess.id) 
			FROM attendance_sessions asess
			JOIN mata_kuliah mk ON asess.course_id = mk.kode
			WHERE mk.semester = 4 AND asess.status = 'active' AND asess.expires_at > NOW()
		`).Scan(&activeSessions)
	} else {
		config.DB.QueryRow(`
			SELECT COUNT(*) 
			FROM attendance_sessions 
			WHERE dosen_id = $1 AND status = 'active' AND expires_at > NOW()
		`, dosenID).Scan(&activeSessions)
	}
	stats["active_sessions"] = activeSessions

	// 5. Today's attendance
	var todayAttendance int
	if !superDosen {
		config.DB.QueryRow(`
			SELECT COUNT(DISTINCT a.id)
			FROM attendance a
			JOIN attendance_sessions asess ON a.session_id = asess.id
			WHERE asess.dosen_id = $1 AND (a.created_at)::date = CURRENT_DATE
		`, dosenID).Scan(&todayAttendance)
	}
	stats["today_attendance"] = todayAttendance

	// 6. Tasks to grade
	var tasksToGrade int
	if superDosen {
		config.DB.QueryRow(`
			SELECT COUNT(*) 
			FROM submissions s
			JOIN tugas t ON s.task_id = t.id
			WHERE s.grade IS NULL OR s.grade = 0
		`).Scan(&tasksToGrade)
	} else {
		config.DB.QueryRow(`
			SELECT COUNT(*) 
			FROM submissions s
			JOIN tugas t ON s.task_id = t.id
			JOIN mata_kuliah mk ON t.course_id = mk.kode
			WHERE mk.dosen_id = $1 AND (s.grade IS NULL OR s.grade = 0)
		`, dosenID).Scan(&tasksToGrade)
	}
	stats["tasks_to_grade"] = tasksToGrade

	// 7. Weekly attendance trend
	var weeklyData []gin.H
	rows, _ := config.DB.Query(`
		SELECT 
			(a.created_at)::date as date,
			COUNT(DISTINCT a.student_id) as student_count,
			COUNT(DISTINCT CASE WHEN a.status = 'present' THEN a.student_id END) as present_count
		FROM attendance a
		JOIN attendance_sessions asess ON a.session_id = asess.id
		WHERE asess.dosen_id = $1 
			AND a.created_at >= CURRENT_DATE - INTERVAL '7 days'
		GROUP BY (a.created_at)::date
		ORDER BY date
	`, dosenID)

	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var date string
			var total, present int
			rows.Scan(&date, &total, &present)
			var percentage float64
			if total > 0 {
				percentage = float64(present) / float64(total) * 100
			}
			weeklyData = append(weeklyData, gin.H{
				"date":       date,
				"total":      total,
				"present":    present,
				"percentage": percentage,
			})
		}
	}
	stats["weekly_attendance"] = weeklyData

	utils.SuccessResponse(c, stats, "Stats retrieved successfully")
}

// GetDosenCourses - Get courses taught by dosen
func GetDosenCourses(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Get dosen ID
	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		log.Printf("Error GetDosenCourses: user_id=%v err=%v", userID, err)
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found untuk user_id ini")
		return
	}

	// Check if superdosen (akses semua matkul)
	superDosen := isSuperDosen(userID)

	// Query courses with schedule and student count
	var query string
	var rows *sql.Rows

	if superDosen {
		// Super dosen: ambil SEMUA matkul semester 4
		query = `
			SELECT 
				mk.kode, mk.nama, mk.sks, mk.hari, mk.jam_mulai, mk.jam_selesai,
				(SELECT COUNT(DISTINCT mmk.mahasiswa_id) FROM mahasiswa_mata_kuliah mmk WHERE mmk.mata_kuliah_kode = mk.kode) as student_count,
				(SELECT COUNT(*) FROM attendance_sessions WHERE course_id = mk.kode AND (created_at)::date = CURRENT_DATE) as today_sessions
			FROM mata_kuliah mk
			WHERE mk.semester = 4
			ORDER BY 
				CASE COALESCE(mk.kategori, 'wajib')
					WHEN 'wajib' THEN 1
					WHEN 'peminatan_cs' THEN 2
					WHEN 'peminatan_ai' THEN 3
				END,
				CASE mk.hari
					WHEN 'Senin' THEN 1 WHEN 'Selasa' THEN 2 WHEN 'Rabu' THEN 3
					WHEN 'Kamis' THEN 4 WHEN 'Jumat' THEN 5 WHEN 'Sabtu' THEN 6
					WHEN 'Minggu' THEN 7
				END, mk.jam_mulai
		`
		rows, err = config.DB.Query(query)
	} else {
		// Dosen biasa: hanya matkul yang diampu
		query = `
			SELECT 
				mk.kode, mk.nama, mk.sks, mk.hari, mk.jam_mulai, mk.jam_selesai,
				(SELECT COUNT(DISTINCT mmk.mahasiswa_id) FROM mahasiswa_mata_kuliah mmk WHERE mmk.mata_kuliah_kode = mk.kode) as student_count,
				(SELECT COUNT(*) FROM attendance_sessions WHERE course_id = mk.kode AND (created_at)::date = CURRENT_DATE) as today_sessions
			FROM mata_kuliah mk
			WHERE mk.dosen_id = $1 AND mk.semester = 4
			ORDER BY 
				CASE mk.hari
					WHEN 'Senin' THEN 1 WHEN 'Selasa' THEN 2 WHEN 'Rabu' THEN 3
					WHEN 'Kamis' THEN 4 WHEN 'Jumat' THEN 5 WHEN 'Sabtu' THEN 6
					WHEN 'Minggu' THEN 7
				END, mk.jam_mulai
		`
		rows, err = config.DB.Query(query, dosenID)
	}
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch courses")
		return
	}
	defer rows.Close()

	var courses []gin.H
	for rows.Next() {
		var kode, nama, hari, jamMulai, jamSelesai string
		var sks int
		var studentCount, todaySessions int64

		err := rows.Scan(&kode, &nama, &sks, &hari, &jamMulai, &jamSelesai, &studentCount, &todaySessions)
		if err != nil {
			log.Printf("Scan error in GetDosenCourses: %v", err)
			continue
		}

		// Check if today is the course day
		today := time.Now().Weekday()
		dayMap := map[time.Weekday]string{
			time.Monday:    "Senin",
			time.Tuesday:   "Selasa",
			time.Wednesday: "Rabu",
			time.Thursday:  "Kamis",
			time.Friday:    "Jumat",
			time.Saturday:  "Sabtu",
			time.Sunday:    "Minggu",
		}

		isToday := dayMap[today] == hari

		courses = append(courses, gin.H{
			"kode":               kode,
			"nama":               nama,
			"sks":                sks,
			"hari":               hari,
			"jam_mulai":          jamMulai,
			"jam_selesai":        jamSelesai,
			"student_count":      studentCount,
			"today_sessions":     todaySessions,
			"is_today":           isToday,
			"can_create_session": isToday && todaySessions < 3, // Max 3 sessions per day
		})
	}

	utils.SuccessResponse(c, courses, "Courses retrieved successfully")
}

/*
// UpdateDosenProfile - Update profile dosen
func UpdateDosenProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var input struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
		Email string `json:"email"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.ValidationError(c, "Invalid input")
		return
	}

	// Update user email if provided
	if input.Email != "" {
		_, err := config.DB.Exec("UPDATE users SET email = $1 WHERE id = $2", input.Email, userID)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update email")
			return
		}
	}

	// Update dosen profile
	query := "UPDATE dosen SET name = $3, phone = $4, updated_at = NOW() WHERE user_id = $5"
	_, err := config.DB.Exec(query, input.Name, input.Phone, userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update profile")
		return
	}

	utils.SuccessResponse(c, nil, "Profile updated successfully")
}
*/

// UploadMateri - Upload materi pembelajaran
func UploadMateri(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	courseID := c.PostForm("course_id")
	pertemuanStr := c.PostForm("pertemuan")
	title := c.PostForm("title")
	desc := c.PostForm("desc")

	if courseID == "" || pertemuanStr == "" || title == "" {
		utils.ValidationError(c, "course_id, pertemuan, dan title wajib diisi")
		return
	}

	// Validasi dosen
	userID, _ := c.Get("user_id")
	var dosenID int
	if err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID); err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen tidak ditemukan")
		return
	}

	// SuperDosen bypass: akses semua matkul
	if !isSuperDosen(userID) {
		var exists bool
		if err := config.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mata_kuliah WHERE kode = $1 AND dosen_id = $2)", courseID, dosenID).Scan(&exists); err != nil || !exists {
			utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengampu mata kuliah ini")
			return
		}
	}

	pertemuan, _ := strconv.Atoi(pertemuanStr)
	if pertemuan < 1 || pertemuan > 16 {
		utils.ValidationError(c, "Pertemuan harus 1-16")
		return
	}

	// File materi opsional
	var uploadID int64
	var filePath interface{}
	if _, _, fErr := c.Request.FormFile("file"); fErr == nil {
		// Upload file ke database via UploadFileToDB jika ada file
		uID, uploadedFilePath, uploadErr := UploadFileToDB(c, "file", dosenID, "dosen", "materi", nil, nil)
		if uploadErr != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, uploadErr.Error())
			return
		}
		uploadID = uID
		filePath = uploadedFilePath
		log.Printf("[Materi] File uploaded to DB: id=%d, url=%s", uploadID, uploadedFilePath)
	} else if fErr != http.ErrMissingFile {
		utils.ErrorResponse(c, http.StatusBadRequest, "Gagal membaca file materi")
		return
	}

	// INSERT ke tabel tugas dengan type 'materi'
	query := `
		INSERT INTO tugas 
		(course_id, pertemuan, title, description, file_tugas, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'materi', NOW(), NOW())
		RETURNING id
	`
	var id int64
	err := config.DB.QueryRow(query, courseID, pertemuan, title, desc, filePath).Scan(&id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal upload materi: "+err.Error())
		return
	}

	// Update related_id di uploads table untuk referensi ke tugas
	if uploadID > 0 {
		config.DB.Exec("UPDATE uploads SET related_id = $1, related_table = 'tugas' WHERE id = $2", id, uploadID)
	}

	utils.SuccessResponse(c, gin.H{
		"id":        id,
		"course_id": courseID,
		"pertemuan": pertemuan,
		"title":     title,
		"file_path": filePath,
	}, "Materi berhasil diupload")
}

// CreateTugas - Create tugas untuk mahasiswa
func CreateTugas(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	courseID := c.PostForm("course_id")
	pertemuanStr := c.PostForm("pertemuan")
	title := c.PostForm("title")
	desc := c.PostForm("desc")
	dueDateStr := c.PostForm("due_date")

	if courseID == "" || pertemuanStr == "" || title == "" || desc == "" {
		utils.ValidationError(c, "course_id, pertemuan, title, dan desc wajib diisi")
		return
	}

	// Validasi dosen
	userID, _ := c.Get("user_id")
	var dosenID int
	if err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID); err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen tidak ditemukan")
		return
	}

	// SuperDosen bypass: akses semua matkul
	if !isSuperDosen(userID) {
		var exists bool
		if err := config.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mata_kuliah WHERE kode = $1 AND dosen_id = $2)", courseID, dosenID).Scan(&exists); err != nil || !exists {
			utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengampu mata kuliah ini")
			return
		}
	}

	pertemuan, _ := strconv.Atoi(pertemuanStr)
	if pertemuan < 1 || pertemuan > 16 {
		utils.ValidationError(c, "Pertemuan harus 1-16")
		return
	}

	var dueDate sql.NullTime
	if dueDateStr != "" {
		if t, err := time.Parse("2006-01-02T15:04", dueDateStr); err == nil {
			dueDate = sql.NullTime{Time: t, Valid: true}
		} else {
			utils.ValidationError(c, "Format due_date salah (gunakan datetime-local)")
			return
		}
	} else {
		// Default 7 hari dari sekarang
		dueDate = sql.NullTime{Time: time.Now().Add(7 * 24 * time.Hour), Valid: true}
	}

	// File tugas opsional — upload ke database (BYTEA)
	var filePath sql.NullString
	var uploadID int64
	if _, _, fErr := c.Request.FormFile("file_tugas"); fErr == nil {
		uID, fileURL, uploadErr := UploadFileToDB(c, "file_tugas", dosenID, "dosen", "tugas_dosen", nil, nil)
		if uploadErr != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, "File tugas: "+uploadErr.Error())
			return
		}
		filePath.String = fileURL
		filePath.Valid = true
		uploadID = uID
		log.Printf("[Tugas] File uploaded to DB: id=%d, url=%s", uploadID, fileURL)
	}

	// Insert ke tabel tugas dengan type 'tugas'
	query := `
		INSERT INTO tugas 
		(course_id, pertemuan, title, description, file_tugas, due_date, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'tugas', NOW(), NOW())
		RETURNING id
	`
	var id int64
	err := config.DB.QueryRow(query, courseID, pertemuan, title, desc, filePath, dueDate).Scan(&id)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal membuat tugas: "+err.Error())
		return
	}

	// Update related_id di uploads table untuk referensi ke tugas
	if uploadID > 0 {
		config.DB.Exec("UPDATE uploads SET related_id = $1, related_table = 'tugas' WHERE id = $2", id, uploadID)
	}

	// ========== OpenClaw: Publish event tugas-created ==========
	// Hanya publish jika type = tugas dan due_date valid
	if dueDate.Valid {
		event := utils.BuildTugasEvent(id, courseID, pertemuan, title, desc, dueDate.Time, dosenID)
		go utils.PublishTugasCreatedEvent(config.DB, event)
	} else {
		utils.LogOpenClawSkip("due_date is null", id)
	}
	// ========== End OpenClaw ==========

	utils.SuccessResponse(c, gin.H{
		"id":          id,
		"course_id":   courseID,
		"pertemuan":   pertemuan,
		"title":       title,
		"description": desc,
		"file_tugas":  filePath.String,
		"due_date":    dueDate.Time.Format("2006-01-02 15:04:05"),
		"created_at":  time.Now().Format("2006-01-02 15:04:05"),
	}, "Tugas berhasil dibuat")
}

// GetTugasSubmissions - Get semua pengumpulan tugas
func GetTugasSubmissions(c *gin.Context) {
	courseID := c.Param("course_id")
	pertemuanStr := c.Query("pertemuan")

	if courseID == "" {
		utils.ValidationError(c, "Course ID is required")
		return
	}

	// Validasi dosen mengampu mata kuliah ini
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	// Check if dosen teaches this course (superdosen bypass)
	if !isSuperDosen(userID) {
		var courseExists bool
		err = config.DB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM mata_kuliah 
				WHERE kode = $1 AND dosen_id = $2
			)
		`, courseID, dosenID).Scan(&courseExists)

		if err != nil || !courseExists {
			utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengampu mata kuliah ini")
			return
		}
	}

	var query string
	var args []interface{}

	if pertemuanStr != "" {
		pertemuan, err := strconv.Atoi(pertemuanStr)
		if err != nil {
			utils.ValidationError(c, "Invalid pertemuan")
			return
		}
		query = `
			SELECT 
				s.id, s.task_id, s.student_id, s.file_url, s.answer_text, 
				s.grade, s.created_at, s.updated_at,
				m.name as student_name, m.nim as student_nim, 
				t.title as task_title, t.pertemuan,
				t.due_date
			FROM submissions s
			JOIN mahasiswa m ON s.student_id = m.id
			JOIN tugas t ON s.task_id = t.id
			WHERE t.course_id = $1 AND t.pertemuan = $2
			ORDER BY s.created_at DESC
		`
		args = []interface{}{courseID, pertemuan}
	} else {
		query = `
			SELECT 
				s.id, s.task_id, s.student_id, s.file_url, s.answer_text, 
				s.grade, s.created_at, s.updated_at,
				m.name as student_name, m.nim as student_nim, 
				t.title as task_title, t.pertemuan,
				t.due_date
			FROM submissions s
			JOIN mahasiswa m ON s.student_id = m.id
			JOIN tugas t ON s.task_id = t.id
			WHERE t.course_id = $1
			ORDER BY t.pertemuan DESC, s.created_at DESC
		`
		args = []interface{}{courseID}
	}

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch submissions: "+err.Error())
		return
	}
	defer rows.Close()

	var submissions []gin.H
	for rows.Next() {
		var id, taskID, studentID, pertemuan int
		var fileURL, answerText sql.NullString
		var grade sql.NullFloat64
		var createdAt, updatedAt time.Time
		var studentName, studentNIM, taskTitle string
		var dueDate sql.NullTime

		err := rows.Scan(&id, &taskID, &studentID, &fileURL, &answerText, &grade, &createdAt,
			&updatedAt, &studentName, &studentNIM, &taskTitle, &pertemuan, &dueDate)
		if err != nil {
			continue
		}

		// Check if submission is late
		var isLate bool
		var lateDays int
		if dueDate.Valid && createdAt.After(dueDate.Time) {
			isLate = true
			lateDays = int(createdAt.Sub(dueDate.Time).Hours() / 24)
			if lateDays < 1 {
				lateDays = 1
			}
		}

		submission := gin.H{
			"id":           id,
			"task_id":      taskID,
			"student_id":   studentID,
			"file_url":     fileURL.String,
			"answer_text":  answerText.String,
			"grade":        grade.Float64,
			"created_at":   createdAt.Format("2006-01-02 15:04:05"),
			"updated_at":   updatedAt.Format("2006-01-02 15:04:05"),
			"student_name": studentName,
			"student_nim":  studentNIM,
			"task_title":   taskTitle,
			"pertemuan":    pertemuan,
			"due_date":     dueDate.Time.Format("2006-01-02 15:04:05"),
			"is_late":      isLate,
			"late_days":    lateDays,
			"graded":       grade.Valid && grade.Float64 > 0,
		}
		submissions = append(submissions, submission)
	}

	// Get statistics
	var totalSubmissions, gradedSubmissions, lateSubmissions int
	config.DB.QueryRow(`
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN s.grade IS NOT NULL AND s.grade > 0 THEN 1 END) as graded,
			COUNT(CASE WHEN s.created_at > t.due_date THEN 1 END) as late
		FROM submissions s
		JOIN tugas t ON s.task_id = t.id
		WHERE t.course_id = $1
	`, courseID).Scan(&totalSubmissions, &gradedSubmissions, &lateSubmissions)

	utils.SuccessResponse(c, gin.H{
		"submissions": submissions,
		"statistics": gin.H{
			"total":   totalSubmissions,
			"graded":  gradedSubmissions,
			"late":    lateSubmissions,
			"pending": totalSubmissions - gradedSubmissions,
		},
	}, "Submissions retrieved successfully")
}

// GradeSubmission - Beri nilai pada pengumpulan tugas
func GradeSubmission(c *gin.Context) {
	submissionIDStr := c.Param("submission_id")
	if submissionIDStr == "" {
		utils.ValidationError(c, "Submission ID is required")
		return
	}

	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		utils.ValidationError(c, "Invalid submission ID")
		return
	}

	var input struct {
		Grade float64 `json:"grade" binding:"required,gte=0,lte=100"`
		Notes string  `json:"notes"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.ValidationError(c, "Invalid input: Grade harus antara 0-100")
		return
	}

	// Validasi dosen memiliki akses
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var dosenID int
	err = config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	// Check if submission belongs to dosen's course (superdosen bypass)
	var courseID string
	if isSuperDosen(userID) {
		err = config.DB.QueryRow(`
			SELECT t.course_id FROM submissions s JOIN tugas t ON s.task_id = t.id WHERE s.id = $1
		`, submissionID).Scan(&courseID)
	} else {
		err = config.DB.QueryRow(`
			SELECT t.course_id 
			FROM submissions s
			JOIN tugas t ON s.task_id = t.id
			JOIN mata_kuliah mk ON t.course_id = mk.kode
			WHERE s.id = $1 AND mk.dosen_id = $2
		`, submissionID, dosenID).Scan(&courseID)
	}

	if err != nil {
		utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak memiliki akses ke pengumpulan ini")
		return
	}

	// Update submission with grade
	query := `UPDATE submissions SET grade = $1, updated_at = NOW() WHERE id = $2`
	result, err := config.DB.Exec(query, input.Grade, submissionID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to grade submission: "+err.Error())
		return
	}

	rowsAffected, _ := result.RowsAffected()

	// Get submission details
	var studentName, taskTitle string
	var studentID int
	config.DB.QueryRow(`
		SELECT m.name, m.id, t.title
		FROM submissions s
		JOIN mahasiswa m ON s.student_id = m.id
		JOIN tugas t ON s.task_id = t.id
		WHERE s.id = $1
	`, submissionID).Scan(&studentName, &studentID, &taskTitle)

	utils.SuccessResponse(c, gin.H{
		"submission_id": submissionID,
		"student_id":    studentID,
		"student_name":  studentName,
		"task_title":    taskTitle,
		"grade":         input.Grade,
		"updated_at":    time.Now().Format("2006-01-02 15:04:05"),
		"rows_affected": rowsAffected,
	}, "Submission graded successfully")
}

// DeleteMateri - Hapus materi
func DeleteMateri(c *gin.Context) {
	materiID := c.Param("id")
	if materiID == "" {
		utils.ValidationError(c, "Materi ID diperlukan")
		return
	}

	// Validasi dosen
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen tidak ditemukan")
		return
	}

	// Cek kepemilikan materi (superdosen bypass)
	var courseID string
	var filePath sql.NullString
	if isSuperDosen(userID) {
		err = config.DB.QueryRow(`
			SELECT t.course_id, t.file_tugas FROM tugas t WHERE t.id = $1 AND t.type = 'materi'
		`, materiID).Scan(&courseID, &filePath)
	} else {
		err = config.DB.QueryRow(`
			SELECT t.course_id, t.file_tugas 
			FROM tugas t
			JOIN mata_kuliah mk ON t.course_id = mk.kode
			WHERE t.id = $1 AND t.type = 'materi' AND mk.dosen_id = $2
		`, materiID, dosenID).Scan(&courseID, &filePath)
	}
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Materi tidak ditemukan atau Anda tidak memiliki akses")
		return
	}

	// Soft-delete file dari uploads table jika ada
	if filePath.Valid && filePath.String != "" {
		// Extract upload UUID from /api/files/{uuid} URL pattern
		var uploadUUIDFromURL string
		if _, scanErr := fmt.Sscanf(filePath.String, "/api/files/%s", &uploadUUIDFromURL); scanErr == nil {
			config.DB.Exec("UPDATE uploads SET deleted_at = NOW() WHERE uuid = $1", uploadUUIDFromURL)
		}
	}

	// Hapus dari database
	_, err = config.DB.Exec("DELETE FROM tugas WHERE id = $1", materiID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal menghapus materi: "+err.Error())
		return
	}

	utils.SuccessResponse(c, nil, "Materi berhasil dihapus")
}

// DeleteTugas - Hapus tugas
func DeleteTugas(c *gin.Context) {
	tugasID := c.Param("id")
	if tugasID == "" {
		utils.ValidationError(c, "Tugas ID diperlukan")
		return
	}

	// Validasi dosen
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var dosenID int
	err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen tidak ditemukan")
		return
	}

	// Cek kepemilikan tugas (superdosen bypass)
	var courseID string
	var filePath sql.NullString
	if isSuperDosen(userID) {
		err = config.DB.QueryRow(`
			SELECT t.course_id, t.file_tugas FROM tugas t WHERE t.id = $1 AND t.type = 'tugas'
		`, tugasID).Scan(&courseID, &filePath)
	} else {
		err = config.DB.QueryRow(`
			SELECT t.course_id, t.file_tugas 
			FROM tugas t
			JOIN mata_kuliah mk ON t.course_id = mk.kode
			WHERE t.id = $1 AND t.type = 'tugas' AND mk.dosen_id = $2
		`, tugasID, dosenID).Scan(&courseID, &filePath)
	}
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Tugas tidak ditemukan atau Anda tidak memiliki akses")
		return
	}

	// Soft-delete file dari uploads table jika ada
	if filePath.Valid && filePath.String != "" {
		var uploadUUIDFromURL string
		if _, scanErr := fmt.Sscanf(filePath.String, "/api/files/%s", &uploadUUIDFromURL); scanErr == nil {
			config.DB.Exec("UPDATE uploads SET deleted_at = NOW() WHERE uuid = $1", uploadUUIDFromURL)
		}
	}

	// Hapus submissions terkait
	_, err = config.DB.Exec("DELETE FROM submissions WHERE task_id = $1", tugasID)
	if err != nil {
		fmt.Printf("Warning: Gagal menghapus submissions: %v\n", err)
	}

	// Hapus tugas dari database
	_, err = config.DB.Exec("DELETE FROM tugas WHERE id = $1", tugasID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal menghapus tugas: "+err.Error())
		return
	}

	utils.SuccessResponse(c, nil, "Tugas dan semua pengumpulan terkait berhasil dihapus")
}

// DeleteSubmission - Hapus pengumpulan tugas oleh dosen
func DeleteSubmission(c *gin.Context) {
	submissionIDStr := c.Param("submission_id")
	if submissionIDStr == "" {
		utils.ValidationError(c, "Submission ID is required")
		return
	}

	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil {
		utils.ValidationError(c, "Invalid submission ID")
		return
	}

	// Validasi dosen
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var dosenID int
	err = config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
		return
	}

	// Cek kepemilikan tugas
	var courseID string
	err = config.DB.QueryRow(`
		SELECT t.course_id 
		FROM submissions s
		JOIN tugas t ON s.task_id = t.id
		WHERE s.id = $1
	`, submissionID).Scan(&courseID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Submission not found")
		return
	}

	// Cek apakah dosen mengampu mata kuliah ini (superdosen bypass)
	if !isSuperDosen(userID) {
		var courseExists bool
		err = config.DB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM mata_kuliah 
				WHERE kode = $1 AND dosen_id = $2
			)
		`, courseID, dosenID).Scan(&courseExists)
		if err != nil || !courseExists {
			utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengampu mata kuliah ini")
			return
		}
	}

	// Hapus file submission jika ada
	var fileURL sql.NullString
	err = config.DB.QueryRow("SELECT file_url FROM submissions WHERE id = $1", submissionID).Scan(&fileURL)
	// Soft-delete file dari uploads table jika ada
	if err == nil && fileURL.Valid && fileURL.String != "" {
		var uploadUUIDFromURL string
		if _, scanErr := fmt.Sscanf(fileURL.String, "/api/files/%s", &uploadUUIDFromURL); scanErr == nil {
			config.DB.Exec("UPDATE uploads SET deleted_at = NOW() WHERE uuid = $1", uploadUUIDFromURL)
		}
	}

	// Hapus submission
	_, err = config.DB.Exec("DELETE FROM submissions WHERE id = $1", submissionID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete submission: "+err.Error())
		return
	}

	utils.SuccessResponse(c, nil, "Submission deleted successfully")
}

// GetPertemuanByMatkul - Untuk dosen dan mahasiswa
func GetPertemuanByMatkul(c *gin.Context) {
	courseID := c.Param("course_id")
	if courseID == "" {
		utils.ValidationError(c, "course_id diperlukan")
		return
	}

	// Untuk dosen: validasi mengampu mata kuliah
	// Untuk mahasiswa: validasi mengambil mata kuliah
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")

	if userRole == "dosen" {
		var dosenID int
		if err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID); err != nil {
			utils.ErrorResponse(c, http.StatusNotFound, "Dosen tidak ditemukan")
			return
		}

		// SuperDosen bypass: akses semua matkul
		if !isSuperDosen(userID) {
			var exists bool
			if err := config.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mata_kuliah WHERE kode = $1 AND dosen_id = $2)", courseID, dosenID).Scan(&exists); err != nil || !exists {
				utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengampu mata kuliah ini")
				return
			}
		}
	} else if userRole == "mahasiswa" {
		var mahasiswaID int
		if err := config.DB.QueryRow("SELECT id FROM mahasiswa WHERE user_id = $1", userID).Scan(&mahasiswaID); err != nil {
			utils.ErrorResponse(c, http.StatusNotFound, "Mahasiswa tidak ditemukan")
			return
		}

		var exists bool
		if err := config.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mahasiswa_mata_kuliah WHERE mata_kuliah_kode = $1 AND mahasiswa_id = $2)", courseID, mahasiswaID).Scan(&exists); err != nil || !exists {
			utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengambil mata kuliah ini")
			return
		}
	}

	rows, err := config.DB.Query(`
		SELECT pertemuan, 
			   COUNT(CASE WHEN type = 'materi' THEN 1 END) > 0 as has_materi,
			   COUNT(CASE WHEN type = 'tugas' THEN 1 END) > 0 as has_tugas
		FROM tugas WHERE course_id = $1 AND deleted_at IS NULL
		GROUP BY pertemuan ORDER BY pertemuan
	`, courseID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil data")
		return
	}
	defer rows.Close()

	list := make([]gin.H, 16)
	for i := range list {
		list[i] = gin.H{"pertemuan": i + 1, "has_materi": false, "has_tugas": false}
	}

	for rows.Next() {
		var p int
		var m, t bool
		rows.Scan(&p, &m, &t)
		if p >= 1 && p <= 16 {
			list[p-1] = gin.H{"pertemuan": p, "has_materi": m, "has_tugas": t}
		}
	}

	utils.SuccessResponse(c, list, "Data pertemuan berhasil diambil")
}

// GetPertemuanDetail - Get detail pertemuan dengan tugas dan materi
func GetPertemuanDetail(c *gin.Context) {
	courseID := c.Param("course_id")
	pertemuanStr := c.Param("pertemuan")

	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	userRole, _ := c.Get("role")

	// Validasi berdasarkan role
	if userRole == "dosen" {
		var dosenID int
		err := config.DB.QueryRow("SELECT id FROM dosen WHERE user_id = $1", userID).Scan(&dosenID)
		if err != nil {
			utils.ErrorResponse(c, http.StatusNotFound, "Dosen not found")
			return
		}

		// Check if dosen teaches this course (superdosen bypass)
		if !isSuperDosen(userID) {
			var courseExists bool
			err = config.DB.QueryRow(`
				SELECT EXISTS(
					SELECT 1 FROM mata_kuliah 
					WHERE kode = $1 AND dosen_id = $2
				)
			`, courseID, dosenID).Scan(&courseExists)

			if err != nil || !courseExists {
				utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengampu mata kuliah ini")
				return
			}
		}
	} else if userRole == "mahasiswa" {
		var mahasiswaID int
		err := config.DB.QueryRow("SELECT id FROM mahasiswa WHERE user_id = $1", userID).Scan(&mahasiswaID)
		if err != nil {
			utils.ErrorResponse(c, http.StatusNotFound, "Mahasiswa not found")
			return
		}

		// Check if mahasiswa takes this course
		var courseExists bool
		err = config.DB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM mahasiswa_mata_kuliah 
				WHERE mata_kuliah_kode = $1 AND mahasiswa_id = $2
			)
		`, courseID, mahasiswaID).Scan(&courseExists)

		if err != nil || !courseExists {
			utils.ErrorResponse(c, http.StatusForbidden, "Anda tidak mengambil mata kuliah ini")
			return
		}
	}

	pertemuan, err := strconv.Atoi(pertemuanStr)
	if err != nil || pertemuan < 1 || pertemuan > 16 {
		utils.ValidationError(c, "Invalid pertemuan")
		return
	}

	query := `
		SELECT id, type, title, description, file_tugas, due_date, created_at 
		FROM tugas 
		WHERE course_id = $1 AND pertemuan = $2 AND deleted_at IS NULL 
		ORDER BY type, created_at
	`

	rows, err := config.DB.Query(query, courseID, pertemuan)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch pertemuan detail")
		return
	}
	defer rows.Close()

	var materi []gin.H
	var tugas []gin.H

	for rows.Next() {
		var id int
		var taskType, title, description, fileTugas sql.NullString
		var dueDate sql.NullTime
		var createdAt time.Time

		err := rows.Scan(&id, &taskType, &title, &description, &fileTugas, &dueDate, &createdAt)
		if err != nil {
			continue
		}

		item := gin.H{
			"id":         id,
			"title":      title.String,
			"desc":       description.String,
			"file_path":  fileTugas.String,
			"created_at": createdAt.Format("2006-01-02 15:04:05"),
		}

		if dueDate.Valid {
			item["due_date"] = dueDate.Time.Format("2006-01-02 15:04:05")
		}

		// Jika user adalah mahasiswa, tambahkan submission status untuk tugas
		if taskType.String == "tugas" && userRole == "mahasiswa" {
			// Get submission status untuk mahasiswa ini
			var mahasiswaID int
			config.DB.QueryRow("SELECT id FROM mahasiswa WHERE user_id = $1", userID).Scan(&mahasiswaID)

			var submission struct {
				ID         int     `json:"id"`
				FileURL    string  `json:"file_url"`
				AnswerText string  `json:"answer_text"`
				Grade      float64 `json:"grade"`
				CreatedAt  string  `json:"created_at"`
			}

			err = config.DB.QueryRow(`
				SELECT id, COALESCE(file_url, ''), COALESCE(answer_text, ''), COALESCE(grade, 0), created_at
				FROM submissions 
				WHERE task_id = $1 AND student_id = $2
			`, id, mahasiswaID).Scan(&submission.ID, &submission.FileURL, &submission.AnswerText, &submission.Grade, &submission.CreatedAt)

			if err == nil && submission.ID > 0 {
				item["submission"] = submission
			} else {
				item["submission"] = nil
			}
		}

		if taskType.String == "materi" {
			materi = append(materi, item)
		} else if taskType.String == "tugas" {
			tugas = append(tugas, item)
		}
	}

	// Get course info
	var courseName, hari, jamMulai, jamSelesai string
	config.DB.QueryRow(`
		SELECT nama, hari, jam_mulai, jam_selesai 
		FROM mata_kuliah 
		WHERE kode = $1
	`, courseID).Scan(&courseName, &hari, &jamMulai, &jamSelesai)

	utils.SuccessResponse(c, gin.H{
		"course_id":   courseID,
		"course_name": courseName,
		"pertemuan":   pertemuan,
		"hari":        hari,
		"jam_mulai":   jamMulai,
		"jam_selesai": jamSelesai,
		"materi":      materi,
		"tugas":       tugas,
		"role":        userRole,
		"fetched_at":  time.Now().Format("2006-01-02 15:04:05"),
	}, "Pertemuan detail retrieved successfully")
}

// GetCourseInfo - Get basic course info for any role
func GetCourseInfo(c *gin.Context) {
	courseID := c.Param("course_id")
	if courseID == "" {
		utils.ValidationError(c, "course_id diperlukan")
		return
	}

	var nama string
	err := config.DB.QueryRow("SELECT nama FROM mata_kuliah WHERE kode = $1", courseID).Scan(&nama)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Mata kuliah tidak ditemukan")
		return
	}

	utils.SuccessResponse(c, gin.H{
		"kode": courseID,
		"nama": nama,
	}, "Course info retrieved")
}
