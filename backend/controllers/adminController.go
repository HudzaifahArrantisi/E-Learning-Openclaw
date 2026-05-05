package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nf-student-hub-backend/config"
	"nf-student-hub-backend/utils"

	"github.com/gin-gonic/gin"
)

// GetAdminProfile - Mendapatkan profile admin
func GetAdminProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Login dulu!")
		return
	}

	// Cek apakah user adalah admin
	var role string
	err := config.DB.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
	if err != nil || role != "admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya admin yang dapat mengakses!")
		return
	}

	// Ambil data dari tabel admin
	var adminData struct {
		ID             int     `json:"id"`
		Name           string  `json:"name"`
		Username       string  `json:"username"`
		Bio            string  `json:"bio"`
		Email          string  `json:"email"`
		ProfilePicture *string `json:"profile_picture"`
		Website        *string `json:"website"`
		Phone          *string `json:"phone"`
	}

	query := `
		SELECT a.id, a.name, a.username, a.bio, u.email, 
		       a.profile_picture, a.website, a.phone
		FROM admin a
		JOIN users u ON a.user_id = u.id
		WHERE a.user_id = $1
	`

	err = config.DB.QueryRow(query, userID).Scan(
		&adminData.ID,
		&adminData.Name,
		&adminData.Username,
		&adminData.Bio,
		&adminData.Email,
		&adminData.ProfilePicture,
		&adminData.Website,
		&adminData.Phone,
	)

	if err != nil {
		// Fallback: jika tidak ada di tabel admin, ambil dari users
		var email string
		config.DB.QueryRow("SELECT email FROM users WHERE id = $1", userID).Scan(&email)
		
		parts := strings.Split(email, "@")
		adminName := "Admin"
		if len(parts) > 0 {
			adminName = "Admin " + strings.Title(parts[0])
		}
		adminUsername := strings.ToLower(parts[0])

		utils.SuccessResponse(c, gin.H{
			"id":              userID,
			"name":            adminName,
			"username":        adminUsername,
			"email":           email,
			"role":            "admin",
			"profile_picture": nil,
			"website":         nil,
			"phone":           nil,
		}, "Admin profile retrieved")
		return
	}

	utils.SuccessResponse(c, gin.H{
		"id":              adminData.ID,
		"name":            adminData.Name,
		"username":        adminData.Username,
		"bio":             adminData.Bio,
		"email":           adminData.Email,
		"role":            "admin",
		"profile_picture": adminData.ProfilePicture,
		"website":         adminData.Website,
		"phone":           adminData.Phone,
	}, "Admin profile retrieved")
}

// === POSTINGAN ADMIN ===
func CreateAdminPost(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Login dulu!")
		return
	}

	// Cek apakah user adalah admin
	var role string
	err := config.DB.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
	if err != nil || role != "admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya admin yang dapat membuat postingan!")
		return
	}

	// Ambil name dan username dari tabel admin
	var authorName, authorUsername string
	err = config.DB.QueryRow("SELECT name, username FROM admin WHERE user_id = $1", userID).Scan(&authorName, &authorUsername)
	if err != nil || authorName == "" || authorUsername == "" {
		// Fallback ke email
		var email string
		config.DB.QueryRow("SELECT email FROM users WHERE id = $1", userID).Scan(&email)
		parts := strings.Split(email, "@")
		authorName = "Admin " + strings.Title(parts[0])
		authorUsername = strings.ToLower(parts[0])
	}

	title := c.PostForm("title")
	content := c.PostForm("content")
	if title == "" || content == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Judul dan konten wajib diisi!")
		return
	}

	// Insert post dulu (tanpa media_url)
	query := `
		INSERT INTO posts (user_id, role, title, content, media_url, author_name, author_username, likes_count, comments_count, created_at)
		VALUES ($1, 'admin', $2, $3, '', $4, $5, 0, 0, NOW())
		RETURNING id
	`
	var postID int64
	err = config.DB.QueryRow(query, userID, title, content, authorName, authorUsername).Scan(&postID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal simpan ke database: "+err.Error())
		return
	}

	// Upload multiple media (carousel) dan insert ke post_media
	uid, _ := userID.(int)
	firstMediaURL, uploadErr := uploadMultipleMedia(c, int(postID), uid, "admin")
	if uploadErr != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, uploadErr.Error())
		return
	}

	// Update posts.media_url dengan URL media pertama (backward compat)
	if firstMediaURL != "" {
		config.DB.Exec("UPDATE posts SET media_url = $1 WHERE id = $2", firstMediaURL, postID)
	}

	utils.SuccessResponse(c, gin.H{
		"id":              postID,
		"title":           title,
		"content":         content,
		"media_url":       firstMediaURL,
		"author_name":     authorName,
		"author_username": authorUsername,
		"role":            "admin",
	}, "Postingan admin berhasil dibuat!")
}

// === GET UNPAID INVOICES ===
func GetUnpaidInvoices(c *gin.Context) {
	query := `
		SELECT ui.id, ui.student_id, ui.amount, ui.uuid, ui.status, ui.created_at,
		       m.name as student_name, m.nim as student_nim
		FROM ukt_invoices ui
		JOIN mahasiswa m ON ui.student_id = m.id
		WHERE ui.status = 'pending'
		ORDER BY ui.created_at DESC
	`

	rows, err := config.DB.Query(query)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch unpaid invoices")
		return
	}
	defer rows.Close()

	var invoices []gin.H
	for rows.Next() {
		var id, studentID int
		var amount float64
		var uuid, status, studentName, studentNIM string
		var createdAt interface{}

		err := rows.Scan(&id, &studentID, &amount, &uuid, &status, &createdAt, &studentName, &studentNIM)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to scan invoice")
			return
		}

		invoices = append(invoices, gin.H{
			"id":           id,
			"student_id":   studentID,
			"amount":       amount,
			"uuid":         uuid,
			"status":       status,
			"created_at":   createdAt,
			"student_name": studentName,
			"student_nim":  studentNIM,
		})
	}

	utils.SuccessResponse(c, invoices, "Unpaid invoices retrieved successfully")
}

// === GET ALL MAHASISWA UKT STATUS (UNTUK ADMIN KEMAHASISWAAN) ===
func GetAllMahasiswaUKTStatus(c *gin.Context) {
	// Cek apakah user sudah login
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Login dulu!")
		return
	}

	// Cek apakah user adalah admin
	var role string
	var userEmail string
	err := config.DB.QueryRow("SELECT role, email FROM users WHERE id = $1", userID).Scan(&role, &userEmail)
	if err != nil || role != "admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya admin yang dapat mengakses!")
		return
	}

	// Hanya admin dengan email kemahasiswaan@nurulfikri.ac.id yang bisa mengakses
	if userEmail != "kemahasiswaan@nurulfikri.ac.id" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya Admin Kemahasiswaan yang dapat mengakses data UKT mahasiswa!")
		return
	}

	rows, err := config.DB.Query(`
		SELECT 
			m.id, 
			m.name, 
			m.nim, 
			COALESCE(m.sisa_ukt, 7000000) as sisa_ukt,
			COALESCE(m.total_ukt_dibayar, 0) as total_dibayar,
			(7000000 - COALESCE(m.sisa_ukt, 7000000)) as sudah_dibayar,
			CASE 
				WHEN COALESCE(m.sisa_ukt, 7000000) = 0 THEN 'LUNAS'
				WHEN COALESCE(m.sisa_ukt, 7000000) = 7000000 THEN 'BELUM BAYAR'
				ELSE 'SEBAGIAN'
			END as status_bayar,
			ROUND((COALESCE(m.total_ukt_dibayar, 0) / 7000000 * 100), 2) as persentase
		FROM mahasiswa m
		ORDER BY m.sisa_ukt ASC, m.name ASC
	`)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil data mahasiswa: "+err.Error())
		return
	}
	defer rows.Close()

	var mahasiswaList []gin.H
	for rows.Next() {
		var id int
		var name, nim, statusBayar string
		var sisaUKT, totalDibayar, sudahDibayar, persentase float64

		err := rows.Scan(&id, &name, &nim, &sisaUKT, &totalDibayar, &sudahDibayar, &statusBayar, &persentase)
		if err != nil {
			continue
		}

		mahasiswaList = append(mahasiswaList, gin.H{
			"id":              id,
			"nama":            name,
			"nim":             nim,
			"sisa_ukt":        sisaUKT,
			"total_dibayar":   totalDibayar,
			"sudah_dibayar":   sudahDibayar,
			"status_bayar":    statusBayar,
			"persentase":      fmt.Sprintf("%.1f%%", persentase),
			"total_ukt":       7000000,
		})
	}

	// Hitung statistik
	var stats struct {
		TotalMahasiswa   int     `json:"total_mahasiswa"`
		TotalLunas       int     `json:"total_lunas"`
		TotalBelumBayar  int     `json:"total_belum_bayar"`
		TotalSebagian    int     `json:"total_sebagian"`
		TotalPendapatan  float64 `json:"total_pendapatan"`
		TotalSisa        float64 `json:"total_sisa"`
	}

	// Query untuk statistik
	err = config.DB.QueryRow(`
		SELECT 
			COUNT(*) as total_mahasiswa,
			SUM(CASE WHEN COALESCE(sisa_ukt, 7000000) = 0 THEN 1 ELSE 0 END) as total_lunas,
			SUM(CASE WHEN COALESCE(sisa_ukt, 7000000) = 7000000 THEN 1 ELSE 0 END) as total_belum_bayar,
			SUM(CASE WHEN COALESCE(sisa_ukt, 7000000) > 0 AND COALESCE(sisa_ukt, 7000000) < 7000000 THEN 1 ELSE 0 END) as total_sebagian,
			SUM(COALESCE(total_ukt_dibayar, 0)) as total_pendapatan,
			SUM(COALESCE(sisa_ukt, 7000000)) as total_sisa
		FROM mahasiswa
	`).Scan(
		&stats.TotalMahasiswa,
		&stats.TotalLunas,
		&stats.TotalBelumBayar,
		&stats.TotalSebagian,
		&stats.TotalPendapatan,
		&stats.TotalSisa,
	)

	if err != nil {
		// Jika error, hitung dari data yang sudah ada
		stats.TotalMahasiswa = len(mahasiswaList)
		for _, m := range mahasiswaList {
			sisaUKT := m["sisa_ukt"].(float64)
			totalDibayar := m["total_dibayar"].(float64)
			stats.TotalPendapatan += totalDibayar
			stats.TotalSisa += sisaUKT
			if sisaUKT == 0 {
				stats.TotalLunas++
			} else if sisaUKT == 7000000 {
				stats.TotalBelumBayar++
			} else {
				stats.TotalSebagian++
			}
		}
	}

	utils.SuccessResponse(c, gin.H{
		"mahasiswa": mahasiswaList,
		"statistik": stats,
	}, "Data UKT semua mahasiswa berhasil diambil")
}

// GetRiwayatPembayaranByMahasiswaID mengembalikan riwayat pembayaran oleh admin untuk mahasiswa tertentu
func GetRiwayatPembayaranByMahasiswaID(c *gin.Context) {
	mahasiswaID := c.Param("mahasiswa_id")

	// Cek apakah user sudah login
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Login dulu!")
		return
	}

	// Cek apakah user adalah admin
	var role string
	var userEmail string
	err := config.DB.QueryRow("SELECT role, email FROM users WHERE id = $1", userID).Scan(&role, &userEmail)
	if err != nil || role != "admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya admin yang dapat mengakses!")
		return
	}

	// Hanya admin dengan email kemahasiswaan@nurulfikri.ac.id yang bisa mengakses
	if userEmail != "kemahasiswaan@nurulfikri.ac.id" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya Admin Kemahasiswaan yang dapat mengakses riwayat pembayaran!")
		return
	}

	rows, err := config.DB.Query(`
		SELECT rp.id, rp.invoice_uuid, rp.metode, rp.nominal, rp.biaya_admin, rp.total_dibayar, rp.status, rp.tanggal,
		       m.name as mahasiswa_name, m.nim
		FROM riwayat_pembayaran rp
		JOIN mahasiswa m ON rp.mahasiswa_id = m.id
		WHERE rp.mahasiswa_id = $1
		ORDER BY rp.tanggal DESC
	`, mahasiswaID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil riwayat pembayaran: "+err.Error())
		return
	}
	defer rows.Close()

	var riwayat []gin.H
	for rows.Next() {
		var id int
		var invoiceUUID, metode, status, mahasiswaName, nim string
		var nominal, biayaAdmin, totalDibayar float64
		var tanggal string

		err := rows.Scan(&id, &invoiceUUID, &metode, &nominal, &biayaAdmin, &totalDibayar, &status, &tanggal, &mahasiswaName, &nim)
		if err != nil {
			continue
		}

		riwayat = append(riwayat, gin.H{
			"id":             id,
			"invoice_uuid":   invoiceUUID,
			"metode":         metode,
			"nominal":        nominal,
			"biaya_admin":    biayaAdmin,
			"total_dibayar":  totalDibayar,
			"status":         status,
			"tanggal":        tanggal,
			"mahasiswa_name": mahasiswaName,
			"nim":            nim,
		})
	}

	utils.SuccessResponse(c, riwayat, "Riwayat pembayaran retrieved")
}

// SendReminder mengirim pengingat pembayaran
func SendReminder(c *gin.Context) {
	mahasiswaID := c.Param("mahasiswa_id")

	// Cek apakah user sudah login
	userID, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Login dulu!")
		return
	}

	// Cek apakah user adalah admin
	var role string
	var userEmail string
	err := config.DB.QueryRow("SELECT role, email FROM users WHERE id = $1", userID).Scan(&role, &userEmail)
	if err != nil || role != "admin" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya admin yang dapat mengirim reminder!")
		return
	}

	// Hanya admin dengan email kemahasiswaan@nurulfikri.ac.id yang bisa mengakses
	if userEmail != "kemahasiswaan@nurulfikri.ac.id" {
		utils.ErrorResponse(c, http.StatusForbidden, "Hanya Admin Kemahasiswaan yang dapat mengirim reminder!")
		return
	}

	var mahasiswa struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		SisaUKT float64 `json:"sisa_ukt"`
	}

	err = config.DB.QueryRow(`
		SELECT m.name, u.email, COALESCE(m.sisa_ukt, 7000000)
		FROM mahasiswa m
		JOIN users u ON m.user_id = u.id
		WHERE m.id = $1
	`, mahasiswaID).Scan(&mahasiswa.Name, &mahasiswa.Email, &mahasiswa.SisaUKT)

	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Mahasiswa tidak ditemukan: "+err.Error())
		return
	}

	// TODO: Implementasi pengiriman email
	// Ini adalah stub implementation
	fmt.Printf("Mengirim email pengingat ke %s (%s) - Sisa UKT: Rp %.0f\n", 
		mahasiswa.Name, mahasiswa.Email, mahasiswa.SisaUKT)

	utils.SuccessResponse(c, mahasiswa.Name, "Pengingat telah dikirim")
}

// =============================================
// SUPER DOSEN ADMIN - KELOLA MATKUL SEMUA
// =============================================

// GetAllCourses - Ambil semua matkul semester 4 (untuk super admin)
func GetAllCourses(c *gin.Context) {
	semesterStr := c.DefaultQuery("semester", "4")
	kategori := c.Query("kategori")

	query := `
		SELECT 
			mk.kode, mk.nama, mk.sks, mk.hari, mk.jam_mulai, mk.jam_selesai,
			mk.semester, COALESCE(mk.kategori, 'wajib') as kategori,
			COALESCE(d.name, 'N/A') as dosen_name,
			(SELECT COUNT(DISTINCT mmk.mahasiswa_id) FROM mahasiswa_mata_kuliah mmk WHERE mmk.mata_kuliah_kode = mk.kode) as student_count
		FROM mata_kuliah mk
		LEFT JOIN dosen d ON mk.dosen_id = d.id
		WHERE mk.semester = $1
	`
	args := []interface{}{semesterStr}

	if kategori != "" {
		query += fmt.Sprintf(" AND mk.kategori = $%d", len(args)+1)
		args = append(args, kategori)
	}

	query += `
		ORDER BY 
			CASE mk.kategori
				WHEN 'wajib' THEN 1
				WHEN 'peminatan_cs' THEN 2
				WHEN 'peminatan_ai' THEN 3
			END,
			mk.kode
	`

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal mengambil data matkul: "+err.Error())
		return
	}
	defer rows.Close()

	var courses []gin.H
	for rows.Next() {
		var kode, nama, hari, jamMulai, jamSelesai, dosenName, kategoriVal string
		var sks, semester, studentCount int

		err := rows.Scan(&kode, &nama, &sks, &hari, &jamMulai, &jamSelesai, &semester, &kategoriVal, &dosenName, &studentCount)
		if err != nil {
			continue
		}

		courses = append(courses, gin.H{
			"kode":          kode,
			"nama":          nama,
			"sks":           sks,
			"hari":          hari,
			"jam_mulai":     jamMulai,
			"jam_selesai":   jamSelesai,
			"semester":      semester,
			"kategori":      kategoriVal,
			"dosen_name":    dosenName,
			"student_count": studentCount,
		})
	}

	utils.SuccessResponse(c, courses, "Semua matkul berhasil diambil")
}

// AdminUploadMateri - Upload materi (akses ke semua matkul)
func AdminUploadMateri(c *gin.Context) {
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

	userID, _ := c.Get("user_id")

	// Verify course exists (no dosen ownership check - admin has access to ALL)
	var exists bool
	if err := config.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mata_kuliah WHERE kode = $1)", courseID).Scan(&exists); err != nil || !exists {
		utils.ErrorResponse(c, http.StatusNotFound, "Mata kuliah tidak ditemukan")
		return
	}

	pertemuan, _ := strconv.Atoi(pertemuanStr)
	if pertemuan < 1 || pertemuan > 16 {
		utils.ValidationError(c, "Pertemuan harus 1-16")
		return
	}

	// File materi opsional
	uid, _ := userID.(int)
	var uploadID int64
	var filePath interface{}
	if _, _, fErr := c.Request.FormFile("file"); fErr == nil {
		uID, uploadedFilePath, uploadErr := UploadFileToDB(c, "file", uid, "admin", "materi", nil, nil)
		if uploadErr != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, uploadErr.Error())
			return
		}
		uploadID = uID
		filePath = uploadedFilePath
	} else if fErr != http.ErrMissingFile {
		utils.ErrorResponse(c, http.StatusBadRequest, "Gagal membaca file materi")
		return
	}

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

	if uploadID > 0 {
		config.DB.Exec("UPDATE uploads SET related_id = $1, related_table = 'tugas' WHERE id = $2", id, uploadID)
	}

	utils.SuccessResponse(c, gin.H{
		"id":        id,
		"course_id": courseID,
		"pertemuan": pertemuan,
		"title":     title,
		"file_path": filePath,
	}, "Materi berhasil diupload oleh admin")
}

// AdminCreateTugas - Buat tugas (akses ke semua matkul)
func AdminCreateTugas(c *gin.Context) {
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

	userID, _ := c.Get("user_id")

	// Verify course exists (no dosen ownership check)
	var exists bool
	if err := config.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM mata_kuliah WHERE kode = $1)", courseID).Scan(&exists); err != nil || !exists {
		utils.ErrorResponse(c, http.StatusNotFound, "Mata kuliah tidak ditemukan")
		return
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
		dueDate = sql.NullTime{Time: time.Now().Add(7 * 24 * time.Hour), Valid: true}
	}

	// File tugas opsional
	var filePath sql.NullString
	var uploadID int64
	uid, _ := userID.(int)
	if _, _, fErr := c.Request.FormFile("file_tugas"); fErr == nil {
		uID, fileURL, uploadErr := UploadFileToDB(c, "file_tugas", uid, "admin", "tugas_admin", nil, nil)
		if uploadErr != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, "File tugas: "+uploadErr.Error())
			return
		}
		filePath.String = fileURL
		filePath.Valid = true
		uploadID = uID
	}

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

	if uploadID > 0 {
		config.DB.Exec("UPDATE uploads SET related_id = $1, related_table = 'tugas' WHERE id = $2", id, uploadID)
	}

	utils.SuccessResponse(c, gin.H{
		"id":          id,
		"course_id":   courseID,
		"pertemuan":   pertemuan,
		"title":       title,
		"description": desc,
		"file_tugas":  filePath.String,
		"due_date":    dueDate.Time.Format("2006-01-02 15:04:05"),
	}, "Tugas berhasil dibuat oleh admin")
}

// AdminGetPertemuanList - Get daftar pertemuan untuk course tertentu (admin akses semua)
func AdminGetPertemuanList(c *gin.Context) {
	courseID := c.Param("course_id")
	if courseID == "" {
		utils.ValidationError(c, "Course ID required")
		return
	}

	type PertemuanInfo struct {
		Pertemuan  int  `json:"pertemuan"`
		HasMateri  bool `json:"has_materi"`
		HasTugas   bool `json:"has_tugas"`
	}

	var pertemuanList []gin.H
	for i := 1; i <= 16; i++ {
		var materiCount, tugasCount int
		config.DB.QueryRow("SELECT COUNT(*) FROM tugas WHERE course_id = $1 AND pertemuan = $2 AND type = 'materi'", courseID, i).Scan(&materiCount)
		config.DB.QueryRow("SELECT COUNT(*) FROM tugas WHERE course_id = $1 AND pertemuan = $2 AND type = 'tugas'", courseID, i).Scan(&tugasCount)

		pertemuanList = append(pertemuanList, gin.H{
			"pertemuan":  i,
			"has_materi": materiCount > 0,
			"has_tugas":  tugasCount > 0,
		})
	}

	utils.SuccessResponse(c, pertemuanList, "Pertemuan list retrieved")
}

// AdminGetPertemuanDetail - Get detail pertemuan (materi + tugas) untuk admin
func AdminGetPertemuanDetail(c *gin.Context) {
	courseID := c.Param("course_id")
	pertemuanStr := c.Param("pertemuan")

	if courseID == "" || pertemuanStr == "" {
		utils.ValidationError(c, "course_id dan pertemuan diperlukan")
		return
	}

	pertemuan, err := strconv.Atoi(pertemuanStr)
	if err != nil || pertemuan < 1 || pertemuan > 16 {
		utils.ValidationError(c, "Pertemuan harus angka 1-16")
		return
	}

	// Get materi
	materiRows, err := config.DB.Query(`
		SELECT id, title, description, file_tugas, created_at 
		FROM tugas 
		WHERE course_id = $1 AND pertemuan = $2 AND type = 'materi'
		ORDER BY created_at DESC
	`, courseID, pertemuan)

	var materiList []gin.H
	if err == nil {
		defer materiRows.Close()
		for materiRows.Next() {
			var id int
			var title string
			var desc sql.NullString
			var filePath sql.NullString
			var createdAt time.Time

			materiRows.Scan(&id, &title, &desc, &filePath, &createdAt)
			materiList = append(materiList, gin.H{
				"id":         id,
				"title":      title,
				"desc":       desc.String,
				"file_path":  filePath.String,
				"created_at": createdAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	// Get tugas
	tugasRows, err := config.DB.Query(`
		SELECT id, title, description, file_tugas, due_date, created_at
		FROM tugas 
		WHERE course_id = $1 AND pertemuan = $2 AND type = 'tugas'
		ORDER BY created_at DESC
	`, courseID, pertemuan)

	var tugasList []gin.H
	if err == nil {
		defer tugasRows.Close()
		for tugasRows.Next() {
			var id int
			var title string
			var desc sql.NullString
			var filePath sql.NullString
			var dueDate sql.NullTime
			var createdAt time.Time

			tugasRows.Scan(&id, &title, &desc, &filePath, &dueDate, &createdAt)

			dueDateStr := ""
			if dueDate.Valid {
				dueDateStr = dueDate.Time.Format("2006-01-02 15:04:05")
			}

			tugasList = append(tugasList, gin.H{
				"id":         id,
				"title":      title,
				"desc":       desc.String,
				"file_path":  filePath.String,
				"due_date":   dueDateStr,
				"created_at": createdAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	utils.SuccessResponse(c, gin.H{
		"materi": materiList,
		"tugas":  tugasList,
	}, "Detail pertemuan berhasil diambil")
}

// AdminDeleteMateri - Hapus materi (admin akses semua)
func AdminDeleteMateri(c *gin.Context) {
	materiID := c.Param("id")
	if materiID == "" {
		utils.ValidationError(c, "Materi ID diperlukan")
		return
	}

	// Verify it's a materi
	var filePath sql.NullString
	err := config.DB.QueryRow("SELECT file_tugas FROM tugas WHERE id = $1 AND type = 'materi'", materiID).Scan(&filePath)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Materi tidak ditemukan")
		return
	}

	// Soft-delete file
	if filePath.Valid && filePath.String != "" {
		var uploadUUIDFromURL string
		if _, scanErr := fmt.Sscanf(filePath.String, "/api/files/%s", &uploadUUIDFromURL); scanErr == nil {
			config.DB.Exec("UPDATE uploads SET deleted_at = NOW() WHERE uuid = $1", uploadUUIDFromURL)
		}
	}

	_, err = config.DB.Exec("DELETE FROM tugas WHERE id = $1", materiID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal menghapus materi: "+err.Error())
		return
	}

	utils.SuccessResponse(c, nil, "Materi berhasil dihapus oleh admin")
}

// AdminDeleteTugas - Hapus tugas (admin akses semua)
func AdminDeleteTugas(c *gin.Context) {
	tugasID := c.Param("id")
	if tugasID == "" {
		utils.ValidationError(c, "Tugas ID diperlukan")
		return
	}

	var filePath sql.NullString
	err := config.DB.QueryRow("SELECT file_tugas FROM tugas WHERE id = $1 AND type = 'tugas'", tugasID).Scan(&filePath)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Tugas tidak ditemukan")
		return
	}

	// Soft-delete file
	if filePath.Valid && filePath.String != "" {
		var uploadUUIDFromURL string
		if _, scanErr := fmt.Sscanf(filePath.String, "/api/files/%s", &uploadUUIDFromURL); scanErr == nil {
			config.DB.Exec("UPDATE uploads SET deleted_at = NOW() WHERE uuid = $1", uploadUUIDFromURL)
		}
	}

	// Delete submissions first
	config.DB.Exec("DELETE FROM submissions WHERE task_id = $1", tugasID)

	_, err = config.DB.Exec("DELETE FROM tugas WHERE id = $1", tugasID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Gagal menghapus tugas: "+err.Error())
		return
	}

	utils.SuccessResponse(c, nil, "Tugas berhasil dihapus oleh admin")
}

// GetAdminStats - Dashboard statistics for admin
func GetAdminStats(c *gin.Context) {
	_, exists := c.Get("user_id")
	if !exists {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var totalMahasiswa, totalDosen, uktBelumBayar, totalUkmOrmawa int

	// 1. Total Mahasiswa
	config.DB.QueryRow("SELECT COUNT(*) FROM mahasiswa").Scan(&totalMahasiswa)

	// 2. Total Dosen
	config.DB.QueryRow("SELECT COUNT(*) FROM dosen").Scan(&totalDosen)

	// 3. UKT Belum Bayar
	config.DB.QueryRow("SELECT COUNT(*) FROM ukt_invoices WHERE status = 'pending'").Scan(&uktBelumBayar)

	// 4. Total UKM & Ormawa (Combined or simplified)
	config.DB.QueryRow("SELECT (SELECT COUNT(*) FROM ukm) + (SELECT COUNT(*) FROM ormawa)").Scan(&totalUkmOrmawa)

	utils.SuccessResponse(c, gin.H{
		"totalMahasiswa":  totalMahasiswa,
		"totalDosen":      totalDosen,
		"uktBelumBayar":   uktBelumBayar,
		"totalUkmOrmawa":  totalUkmOrmawa,
	}, "Admin statistics retrieved")
}
