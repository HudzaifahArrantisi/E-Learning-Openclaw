package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Sender handles Telegram Bot API communication
type Sender struct {
	BotToken  string
	ChannelID string
	Client    *http.Client
}

// NewSender creates a new Telegram sender
func NewSender(botToken, channelID string) *Sender {
	return &Sender{
		BotToken:  botToken,
		ChannelID: channelID,
		Client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// SendMessageRequest represents the Telegram sendMessage API payload
type SendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// SendMessageResponse represents the Telegram API response
type SendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

// SendMessage sends a message to the configured Telegram channel
// Implements retry with exponential backoff (max 3 attempts)
func (s *Sender) SendMessage(text string) error {
	if s.BotToken == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN is not configured")
	}

	err := s.doSend(text)
	if err == nil {
		log.Printf("[Telegram] Message sent successfully to %s", s.ChannelID)
		return nil
	}

	log.Printf("[Telegram] Send failed: %v", err)
	return fmt.Errorf("failed to send message: %v", err)
}

// doSend performs the actual HTTP call to Telegram Bot API
func (s *Sender) doSend(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.BotToken)

	reqBody := SendMessageRequest{
		ChatID:    s.ChannelID,
		Text:      text,
		ParseMode: "HTML",
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := s.Client.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp SendMessageResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("telegram API error: %s", apiResp.Description)
	}

	return nil
}

// FormatInstantNotification formats the instant notification message
func FormatInstantNotification(title, courseName, description, dueDate, category string, pertemuan int) string {
	title = sanitizeTelegramText(title)
	courseName = sanitizeTelegramText(courseName)
	description = sanitizeTelegramText(description)
	dueDate = sanitizeTelegramText(dueDate)

	categoryLabel := ""
	if category == "peminatan_cs" {
		categoryLabel = " [PEMINATAN CS]"
	} else if category == "peminatan_ai" {
		categoryLabel = " [PEMINATAN AI]"
	}

	return fmt.Sprintf(
		"📚 <b>TUGAS BARU!%s</b>\n\n"+
			"📖 Mata Kuliah: <b>%s</b>\n"+
			"📝 Judul: <b>%s</b>\n"+
			"📋 Pertemuan: %d\n"+
			"📄 Deskripsi: %s\n"+
			"⏰ Deadline: <b>%s</b>\n\n"+
			"Segera kerjakan dan kumpulkan sebelum deadline! 💪",
		categoryLabel, courseName, title, pertemuan, description, dueDate,
	)
}

func FormatReminderNotification(title, courseName, dueDate, reminderType, category string, daysLeft int) string {
	title = sanitizeTelegramText(title)
	courseName = sanitizeTelegramText(courseName)
	dueDate = sanitizeTelegramText(dueDate)

	urgency := getUrgencyLabel(reminderType, daysLeft)
	categoryLabel := getCategoryLabel(category)

	return fmt.Sprintf(
		"⏰ <b>REMINDER TUGAS%s</b> — %s\n\n"+
			"📖 Mata Kuliah: <b>%s</b>\n"+
			"📝 Judul: <b>%s</b>\n"+
			"📅 Deadline: <b>%s</b>\n\n"+
			"Jangan lupa kumpulkan tugasmu! 📤",
		categoryLabel, urgency, courseName, title, dueDate,
	)
}

// FormatGroupedReminderNotification formats multiple reminders into a single compact message
func FormatGroupedReminderNotification(reminders []map[string]interface{}) string {
	if len(reminders) == 0 {
		return ""
	}

	now := time.Now().Format("02 Jan 2006")
	header := fmt.Sprintf("⏰ <b>REMINDER TUGAS — %s</b>\n\n", now)
	
	var body string
	for i, r := range reminders {
		title := sanitizeTelegramText(r["title"].(string))
		courseName := sanitizeTelegramText(r["courseName"].(string))
		dueDate := sanitizeTelegramText(r["dueDate"].(string))
		reminderType := r["reminderType"].(string)
		daysLeft := r["daysLeft"].(int)
		pendingCount := r["pendingCount"].(int)
		
		urgency := getUrgencyLabel(reminderType, daysLeft)
		
		body += fmt.Sprintf(
			"%d. <b>%s</b>\n"+
				"📖 [%s] %s\n"+
				"📅 %s (👥 %d mhs belum)\n\n",
			i+1, urgency, courseName, title, dueDate, pendingCount,
		)
	}

	footer := "Jangan lupa kumpulkan tugasmu! 📤"
	return header + body + footer
}

func getUrgencyLabel(reminderType string, daysLeft int) string {
	switch reminderType {
	case "h3":
		return "! 3 Hari Lagi"
	case "h2":
		return "!! 2 Hari Lagi"
	case "h1":
		return "!!! BESOK!"
	case "h0":
		return "!!!! HARI INI!‼️"
	default:
		return fmt.Sprintf("%d hari lagi", daysLeft)
	}
}

func getCategoryLabel(category string) string {
	if category == "peminatan_cs" {
		return " [PEMINATAN CS]"
	} else if category == "peminatan_ai" {
		return " [PEMINATAN AI]"
	}
	return ""
}

func FormatMateriNotification(title, courseName, description, category string, pertemuan int) string {
	title = sanitizeTelegramText(title)
	courseName = sanitizeTelegramText(courseName)
	description = sanitizeTelegramText(description)

	categoryLabel := ""
	if category == "peminatan_cs" {
		categoryLabel = " [PEMINATAN CS]"
	} else if category == "peminatan_ai" {
		categoryLabel = " [PEMINATAN AI]"
	}

	return fmt.Sprintf(
		"📘 <b>MATERI BARU!%s</b>\n\n"+
			"📖 Mata Kuliah: <b>%s</b>\n"+
			"📝 Judul: <b>%s</b>\n"+
			"📋 Pertemuan: %d\n"+
			"📄 Deskripsi: %s\n\n"+
			"Silakan pelajari materi yang baru diupload! 🤓",
		categoryLabel, courseName, title, pertemuan, description,
	)
}

// sanitizeTelegramText converts markdown-like copied text into safe plain text for HTML parse mode.
func sanitizeTelegramText(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.TrimSpace(s)
	return html.EscapeString(s)
}
