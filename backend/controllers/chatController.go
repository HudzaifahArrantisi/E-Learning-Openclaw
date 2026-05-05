package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nf-student-hub-backend/handlers"
	"nf-student-hub-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ChatController struct {
	db  *gorm.DB
	hub *handlers.WebSocketHub
}

func NewChatController(db *gorm.DB, hub *handlers.WebSocketHub) *ChatController {
	return &ChatController{
		db:  db,
		hub: hub,
	}
}

// helpers to safely extract user info from context
func getUserIDFromContext(c *gin.Context) (int, bool) {
	uidRaw, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	switch v := uidRaw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case uint:
		return int(v), true
	default:
		return 0, false
	}
}

func getUserRoleFromContext(c *gin.Context) string {
	if r, ok := c.Get("user_role"); ok {
		if rs, ok := r.(string); ok {
			return rs
		}
	}
	if r, ok := c.Get("role"); ok {
		if rs, ok := r.(string); ok {
			return rs
		}
	}
	return ""
}

// GetConversations - Get all conversations for current user
func (cc *ChatController) GetConversations(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	var conversations []models.Conversation

	// Query conversations where user is participant
	if err := cc.db.Model(&models.Conversation{}).
		Preload("Participants.User.Mahasiswa").
		Preload("Participants.User.Dosen").
		Preload("Participants.User.Ukm").
		Preload("Participants.User.Ormawa").
		Joins("JOIN conversation_participants ON conversation_participants.conversation_id = conversations.id").
		Where("conversation_participants.user_id = ?", userID).
		Order("conversations.updated_at DESC").
		Find(&conversations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get conversations: " + err.Error(),
		})
		return
	}

	// Get last messages for all these conversations in one go
	var convIDs []int
	for _, conv := range conversations {
		convIDs = append(convIDs, conv.ID)
	}

	lastMessages := make(map[int]models.Message)
	if len(convIDs) > 0 {
		var msgs []models.Message
		cc.db.Where("id IN (SELECT MAX(id) FROM messages WHERE conversation_id IN ? AND deleted_at IS NULL GROUP BY conversation_id)", convIDs).
			Preload("Sender").
			Find(&msgs)
		for _, m := range msgs {
			lastMessages[m.ConversationID] = m
		}
	}

	// Get unread counts for all conversations in one go
	unreadCounts := make(map[int]int64)
	if len(convIDs) > 0 {
		type CountResult struct {
			ConversationID int
			Count          int64
		}
		var results []CountResult
		cc.db.Model(&models.Message{}).
			Select("conversation_id, count(*) as count").
			Where("conversation_id IN ? AND sender_id != ? AND is_read = false AND deleted_at IS NULL", convIDs, userID).
			Group("conversation_id").
			Scan(&results)
		for _, r := range results {
			unreadCounts[r.ConversationID] = r.Count
		}
	}

	// Get pinned conversations for user
	var pinnedConversations []models.PinnedConversation
	cc.db.Where("user_id = ?", userID).Find(&pinnedConversations)
	pinnedMap := make(map[int]bool)
	for _, pc := range pinnedConversations {
		pinnedMap[pc.ConversationID] = true
	}

	// Build response
	response := make([]models.ConversationResponse, 0, len(conversations))
	for _, conv := range conversations {
		// Map participants
		participantResponses := make([]models.ParticipantResponse, 0, len(conv.Participants))
		for _, p := range conv.Participants {
			participantResponses = append(participantResponses, models.ParticipantResponse{
				UserID:     p.UserID,
				User:       models.MapUserToResponse(p.User),
				Role:       p.Role,
				JoinedAt:   p.JoinedAt,
				LastReadAt: p.LastReadAt,
			})
		}

		// Map last message
		var lastMsgResp *models.MessageResponse
		if msg, ok := lastMessages[conv.ID]; ok {
			lastMsgResp = &models.MessageResponse{
				ID:          msg.ID,
				Sender:      models.MapUserToResponse(msg.Sender),
				Content:     msg.Content,
				MessageType: msg.MessageType,
				FileURL:     msg.FileURL,
				FileName:    msg.FileName,
				FileSize:    msg.FileSize,
				IsRead:      msg.IsRead,
				CreatedAt:   msg.CreatedAt,
			}
		}

		response = append(response, models.ConversationResponse{
			ID:           conv.ID,
			Type:         conv.Type,
			Name:         conv.Name,
			LastMessage:  lastMsgResp,
			UnreadCount:  int(unreadCounts[conv.ID]),
			Participants: participantResponses,
			IsPinned:     pinnedMap[conv.ID],
			CreatedAt:    conv.CreatedAt,
			UpdatedAt:    conv.UpdatedAt,
		})
	}


	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"count":   len(response),
	})
}

// GetConversationDetail - Get detailed information about a conversation
func (cc *ChatController) GetConversationDetail(c *gin.Context) {
	conversationID, err := strconv.Atoi(c.Param("conversation_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid conversation ID",
		})
		return
	}

	// get user id
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	// Check if user is participant
	var participant models.ConversationParticipant
	if err := cc.db.Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		First(&participant).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "You are not a participant in this conversation",
		})
		return
	}

	// Get conversation
	var conversation models.Conversation
	if err := cc.db.First(&conversation, conversationID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Conversation not found",
		})
		return
	}

	// Get participants
	var participants []models.ConversationParticipant
	cc.db.Where("conversation_id = ?", conversationID).
		Preload("User").
		Preload("User.Mahasiswa").
		Preload("User.Dosen").
		Preload("User.Ukm").
		Preload("User.Ormawa").
		Find(&participants)

	// Get unread count
	var unreadCount int64
	cc.db.Model(&models.Message{}).
		Where("conversation_id = ? AND sender_id != ? AND is_read = false AND deleted_at IS NULL", conversationID, userID).
		Count(&unreadCount)

	// Get last message
	var lastMessage models.Message
	cc.db.Where("conversation_id = ? AND deleted_at IS NULL", conversationID).
		Order("created_at DESC").
		Preload("Sender").
		First(&lastMessage)

	// Check if pinned
	var isPinned bool
	var pinnedCount int64
	cc.db.Model(&models.PinnedConversation{}).
		Where("user_id = ? AND conversation_id = ?", userID, conversationID).
		Count(&pinnedCount)
	isPinned = pinnedCount > 0

	// Map participants
	participantResponses := make([]models.ParticipantResponse, 0)
	for _, p := range participants {
		participantResponses = append(participantResponses, models.ParticipantResponse{
			UserID:     p.UserID,
			User:       models.MapUserToResponse(p.User),
			Role:       p.Role,
			JoinedAt:   p.JoinedAt,
			LastReadAt: p.LastReadAt,
		})
	}

	// Map mata kuliah if exists
	var mataKuliah *models.MataKuliahResponse
	if conversation.MataKuliahID != nil {
		var mk models.MataKuliah
		if err := cc.db.Preload("Dosen.User").First(&mk, *conversation.MataKuliahID).Error; err == nil {
			dosen := models.MapUserToResponse(mk.Dosen.User)
			mataKuliah = &models.MataKuliahResponse{
				ID:    mk.ID,
				Kode:  mk.Kode,
				Nama:  mk.Nama,
				SKS:   mk.SKS,
				Dosen: dosen,
			}
		}
	}

	// Map last message
	var lastMsgResp *models.MessageResponse
	if lastMessage.ID != 0 {
		lastMsgResp = &models.MessageResponse{
			ID:          lastMessage.ID,
			Sender:      models.MapUserToResponse(lastMessage.Sender),
			Content:     lastMessage.Content,
			MessageType: lastMessage.MessageType,
			FileURL:     lastMessage.FileURL,
			FileName:    lastMessage.FileName,
			FileSize:    lastMessage.FileSize,
			IsRead:      lastMessage.IsRead,
			CreatedAt:   lastMessage.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": models.ConversationResponse{
			ID:           conversation.ID,
			Type:         conversation.Type,
			Name:         conversation.Name,
			MataKuliah:   mataKuliah,
			LastMessage:  lastMsgResp,
			UnreadCount:  int(unreadCount),
			Participants: participantResponses,
			IsPinned:     isPinned,
			CreatedAt:    conversation.CreatedAt,
			UpdatedAt:    conversation.UpdatedAt,
		},
	})
}

// CreateConversation - Create new conversation
func (cc *ChatController) CreateConversation(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}
	userRole := getUserRoleFromContext(c)

	var req models.CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Validate participants
	if len(req.Participants) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "At least 1 participant required",
		})
		return
	}

	// Check if all participants exist
	for _, participantID := range req.Participants {
		var user models.User
		if err := cc.db.First(&user, participantID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("User with ID %d not found", participantID),
			})
			return
		}
	}

	// For private conversations, check if already exists
	if req.Type == "private" && len(req.Participants) == 1 {
		var existingCount int64
		cc.db.Raw(`
			SELECT COUNT(DISTINCT c.id) 
			FROM conversations c
			JOIN conversation_participants cp1 ON cp1.conversation_id = c.id
			JOIN conversation_participants cp2 ON cp2.conversation_id = c.id
			WHERE c.type = 'private' 
			AND cp1.user_id = ? 
			AND cp2.user_id = ?
		`, userID, req.Participants[0]).Scan(&existingCount)

		if existingCount > 0 {
			// Get the existing conversation
			var existingConv models.Conversation
			cc.db.Raw(`
				SELECT c.* 
				FROM conversations c
				JOIN conversation_participants cp1 ON cp1.conversation_id = c.id
				JOIN conversation_participants cp2 ON cp2.conversation_id = c.id
				WHERE c.type = 'private' 
				AND cp1.user_id = ? 
				AND cp2.user_id = ?
				LIMIT 1
			`, userID, req.Participants[0]).Scan(&existingConv)

			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Conversation already exists",
				"data":    existingConv.ID,
			})
			return
		}
	}

	// Start transaction
	tx := cc.db.Begin()

	// Get conversation name
	convName := cc.getConversationName(req.Type, req.Name, userID, req.Participants)

	// Create conversation
	conversation := models.Conversation{
		Type:         req.Type,
		Name:         convName,
		MataKuliahID: req.MataKuliahID,
		CreatedBy:    userID,
	}

	if err := tx.Create(&conversation).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create conversation",
		})
		return
	}

	// Add participants (including creator)
	allParticipants := append([]int{userID}, req.Participants...)
	for _, participantID := range allParticipants {
		role := "member"
		if participantID == userID {
			role = "owner"
		} else if userRole == "dosen" && req.Type == "group" && req.MataKuliahID != nil {
			role = "admin"
		}

		participant := models.ConversationParticipant{
			ConversationID: conversation.ID,
			UserID:         participantID,
			Role:           role,
		}

		if err := tx.Create(&participant).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to add participant",
			})
			return
		}
	}

	// Add system message
	systemMessage := models.Message{
		ConversationID: conversation.ID,
		SenderID:       userID,
		MessageType:    "system",
		Content:        fmt.Sprintf("%s conversation created", strings.Title(req.Type)),
	}

	if err := tx.Create(&systemMessage).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create system message",
		})
		return
	}

	tx.Commit()

	// Broadcast new_conversation event to all participants via WebSocket
	for _, participantID := range allParticipants {
		cc.hub.BroadcastToUser(participantID, models.WebsocketMessage{
			Type: "new_conversation",
			Data: gin.H{
				"conversation_id": conversation.ID,
				"created_by":      userID,
			},
		})
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Conversation created successfully",
		"data":    conversation.ID,
	})
}

// GetMessages - Get messages in a conversation
func (cc *ChatController) GetMessages(c *gin.Context) {
	conversationID, err := strconv.Atoi(c.Param("conversation_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid conversation ID",
		})
		return
	}

	// get user id
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	// Check if user is participant
	var participant models.ConversationParticipant
	if err := cc.db.Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		First(&participant).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "You are not a participant in this conversation",
		})
		return
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	// Get total count
	var totalCount int64
	cc.db.Model(&models.Message{}).
		Where("conversation_id = ? AND deleted_at IS NULL", conversationID).
		Count(&totalCount)

	// Get messages
	var messages []models.Message
	err = cc.db.Where("conversation_id = ? AND deleted_at IS NULL", conversationID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Preload("Sender").
		Preload("Sender.Mahasiswa").
		Preload("Sender.Dosen").
		Find(&messages).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get messages",
		})
		return
	}

	// Reverse for chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	// Mark messages as read in background
	go cc.markMessagesAsRead(conversationID, userID)

	// Build response
	var response []models.MessageResponse
	for _, msg := range messages {
		response = append(response, models.MessageResponse{
			ID:          msg.ID,
			Sender:      models.MapUserToResponse(msg.Sender),
			Content:     msg.Content,
			MessageType: msg.MessageType,
			FileURL:     msg.FileURL,
			FileName:    msg.FileName,
			FileSize:    msg.FileSize,
			IsRead:      msg.IsRead,
			CreatedAt:   msg.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"pagination": gin.H{
			"page":       page,
			"limit":      limit,
			"total":      totalCount,
			"totalPages": (int(totalCount) + limit - 1) / limit,
		},
	})
}

// SendMessage - Send a message
func (cc *ChatController) SendMessage(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	var req models.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Check if user is participant
	var participant models.ConversationParticipant
	if err := cc.db.Where("conversation_id = ? AND user_id = ?", req.ConversationID, userID).
		First(&participant).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "You are not a participant in this conversation",
		})
		return
	}

	// Create message
	message := models.Message{
		ConversationID: req.ConversationID,
		SenderID:       userID,
		MessageType:    req.MessageType,
		Content:        req.Content,
		FileURL:        req.FileURL,
		FileName:       req.FileName,
		FileSize:       req.FileSize,
	}

	if err := cc.db.Create(&message).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to send message",
		})
		return
	}

	// Update conversation timestamp
	cc.db.Model(&models.Conversation{}).
		Where("id = ?", req.ConversationID).
		Update("updated_at", time.Now())

	// Get sender info
	var sender models.User
	cc.db.Preload("Mahasiswa").Preload("Dosen").First(&sender, userID)

	// Build response
	messageResponse := models.MessageResponse{
		ID:          message.ID,
		Sender:      models.MapUserToResponse(sender),
		Content:     message.Content,
		MessageType: message.MessageType,
		FileURL:     message.FileURL,
		FileName:    message.FileName,
		FileSize:    message.FileSize,
		IsRead:      false,
		CreatedAt:   message.CreatedAt,
	}

	// Broadcast via WebSocket
	cc.hub.BroadcastToConversation(req.ConversationID, models.WebsocketMessage{
		Type: "new_message",
		Data: gin.H{
			"conversation_id": req.ConversationID,
			"message":         messageResponse,
		},
	})

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    messageResponse,
	})
}

// DeleteMessage - Delete a message (unsend)
func (cc *ChatController) DeleteMessage(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	messageID := c.Param("message_id")
	var message models.Message

	// Find the message
	if err := cc.db.Where("id = ? AND deleted_at IS NULL", messageID).First(&message).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Message not found"})
		return
	}

	// Check ownership (only sender can delete)
	if message.SenderID != userID {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "You can only delete your own messages"})
		return
	}

	// Perform soft delete
	now := time.Now()
	if err := cc.db.Model(&message).Update("deleted_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to delete message"})
		return
	}

	// Broadcast deletion
	cc.hub.BroadcastToConversation(message.ConversationID, models.WebsocketMessage{
		Type: "message_deleted",
		Data: gin.H{
			"conversation_id": message.ConversationID,
			"message_id":      message.ID,
		},
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message deleted successfully",
	})
}

// CreateMatkulGroup - Create group chat for mata kuliah
func (cc *ChatController) CreateMatkulGroup(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}
	userRole := getUserRoleFromContext(c)

	if userRole != "dosen" && userRole != "admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Only dosen or admin can create mata kuliah groups",
		})
		return
	}

	var req models.CreateGroupMatkulRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Check if mata kuliah exists
	var mataKuliah models.MataKuliah
	if err := cc.db.Preload("Dosen.User").First(&mataKuliah, req.MataKuliahID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Mata kuliah not found",
		})
		return
	}

	// Check if dosen is the teacher
	if userRole == "dosen" {
		if mataKuliah.DosenID == nil || *mataKuliah.DosenID != userID {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "You are not the teacher of this mata kuliah",
			})
			return
		}
	}

	// Check if group already exists
	var existingGroup models.MataKuliahChatGroup
	if err := cc.db.Where("mata_kuliah_id = ? AND deleted_at IS NULL", req.MataKuliahID).First(&existingGroup).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "Group for this mata kuliah already exists",
		})
		return
	}

	// Start transaction
	tx := cc.db.Begin()

	// Create conversation
	conversation := models.Conversation{
		Type:         "group",
		Name:         fmt.Sprintf("%s - %s", mataKuliah.Kode, mataKuliah.Nama),
		MataKuliahID: &mataKuliah.ID,
		CreatedBy:    userID,
	}

	if err := tx.Create(&conversation).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create conversation",
		})
		return
	}

	// Add dosen as owner
	if mataKuliah.DosenID != nil {
		dosenParticipant := models.ConversationParticipant{
			ConversationID: conversation.ID,
			UserID:         *mataKuliah.DosenID,
			Role:           "owner",
		}

		if err := tx.Create(&dosenParticipant).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to add dosen to group",
			})
			return
		}
	}

	// Get all mahasiswa in this course
	var mahasiswaList []models.Mahasiswa
	tx.Model(&models.MahasiswaMataKuliah{}).
		Select("mahasiswa.*").
		Joins("JOIN mahasiswa ON mahasiswa.id = mahasiswa_mata_kuliah.mahasiswa_id").
		Where("mahasiswa_mata_kuliah.mata_kuliah_id = ?", mataKuliah.ID).
		Find(&mahasiswaList)

	// Add all mahasiswa as members
	for _, mhs := range mahasiswaList {
		participant := models.ConversationParticipant{
			ConversationID: conversation.ID,
			UserID:         mhs.UserID,
			Role:           "member",
		}
		tx.Create(&participant)
	}

	// Create mata kuliah chat group record
	matkulGroup := models.MataKuliahChatGroup{
		MataKuliahID:   mataKuliah.ID,
		ConversationID: conversation.ID,
		CreatedBy:      userID,
	}

	if err := tx.Create(&matkulGroup).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create mata kuliah group record",
		})
		return
	}

	// Add system message
	systemMessage := models.Message{
		ConversationID: conversation.ID,
		SenderID:       userID,
		MessageType:    "system",
		Content:        fmt.Sprintf("Mata kuliah group %s created by %s", mataKuliah.Nama, mataKuliah.Dosen.User.Name),
	}

	if err := tx.Create(&systemMessage).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create system message",
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Mata kuliah group created successfully",
		"data": gin.H{
			"conversation_id": conversation.ID,
			"mata_kuliah": gin.H{
				"id":   mataKuliah.ID,
				"nama": mataKuliah.Nama,
				"kode": mataKuliah.Kode,
			},
		},
	})
}

// GetMatkulGroups - List mata kuliah chat groups accessible to the user
func (cc *ChatController) GetMatkulGroups(c *gin.Context) {
	// get user info
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}
	role := getUserRoleFromContext(c)

	var groups []models.MataKuliahChatGroup

	switch role {
	case "dosen":
		// groups where the mata_kuliah is taught by this dosen
		cc.db.Joins("MataKuliah").Where("mata_kuliah.dosen_id = ? AND mata_kuliah_chat_groups.deleted_at IS NULL", userID).
			Preload("MataKuliah").Preload("Conversation").Find(&groups)
	case "mahasiswa":
		// groups for mata_kuliah where this mahasiswa is enrolled
		cc.db.Model(&models.MataKuliahChatGroup{}).
			Joins("JOIN mata_kuliah ON mata_kuliah.id = mata_kuliah_chat_groups.mata_kuliah_id").
			Joins("JOIN mahasiswa_mata_kuliah mm ON mm.mata_kuliah_id = mata_kuliah.id").
			Joins("JOIN mahasiswa m ON m.id = mm.mahasiswa_id").
			Where("m.user_id = ? AND mata_kuliah_chat_groups.deleted_at IS NULL", userID).
			Preload("MataKuliah").Preload("Conversation").Find(&groups)
	case "admin":
		cc.db.Where("deleted_at IS NULL").Preload("MataKuliah").Preload("Conversation").Find(&groups)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid user role"})
		return
	}

	// build simple response
	var resp []gin.H
	for _, g := range groups {
		resp = append(resp, gin.H{
			"id":              g.ID,
			"mata_kuliah_id":  g.MataKuliahID,
			"conversation_id": g.ConversationID,
			"created_by":      g.CreatedBy,
			"created_at":      g.CreatedAt,
			"mata_kuliah": gin.H{
				"id":   g.MataKuliah.ID,
				"kode": g.MataKuliah.Kode,
				"nama": g.MataKuliah.Nama,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp, "count": len(resp)})
}

// GetContacts - Get available contacts for chat
func (cc *ChatController) GetContacts(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}
	userRole := getUserRoleFromContext(c)

	var users []models.User
	var err error

	switch userRole {
	case "mahasiswa":
		// Mahasiswa can chat with dosen (their teachers) and admin
		err = cc.db.Joins("JOIN dosen ON dosen.user_id = users.id").
			Where("users.role = 'dosen' OR users.role = 'admin'").
			Preload("Dosen").
			Preload("Mahasiswa").
			Find(&users).Error

	case "dosen":
		// Dosen can chat with their students, other dosen, and admin
		var mahasiswaIDs []int
		cc.db.Model(&models.MahasiswaMataKuliah{}).
			Select("mahasiswa.user_id").
			Joins("JOIN mata_kuliah ON mata_kuliah.id = mahasiswa_mata_kuliah.mata_kuliah_id").
			Joins("JOIN mahasiswa ON mahasiswa.id = mahasiswa_mata_kuliah.mahasiswa_id").
			Where("mata_kuliah.dosen_id = ?", userID).
			Group("mahasiswa.user_id").
			Pluck("mahasiswa.user_id", &mahasiswaIDs)

		if len(mahasiswaIDs) > 0 {
			err = cc.db.Where("id IN (?)", mahasiswaIDs).
				Or("(role = 'dosen' AND id != ?) OR role = 'admin'", userID).
				Preload("Dosen").
				Preload("Mahasiswa").
				Find(&users).Error
		} else {
			err = cc.db.Where("(role = 'dosen' AND id != ?) OR role = 'admin'", userID).
				Preload("Dosen").
				Preload("Mahasiswa").
				Find(&users).Error
		}

	case "admin":
		// Admin can chat with all dosen and mahasiswa
		err = cc.db.Where("role IN ('dosen', 'mahasiswa')").
			Preload("Dosen").
			Preload("Mahasiswa").
			Find(&users).Error

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid user role",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get contacts: " + err.Error(),
		})
		return
	}

	// Map to response
	var response []models.UserResponse
	for _, user := range users {
		response = append(response, models.MapUserToResponse(user))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"count":   len(response),
	})
}

// SearchUsers - Search users by name/email with role filter (for New Chat)
func (cc *ChatController) SearchUsers(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	query := strings.TrimSpace(c.Query("q"))
	roleFilter := strings.TrimSpace(c.Query("role"))

	// Build base query - exclude current user and deleted users
	dbQuery := cc.db.Model(&models.User{}).
		Where("users.id != ?", userID).
		Where("users.deleted_at IS NULL")

	// Apply role filter if provided
	validRoles := map[string]bool{"admin": true, "mahasiswa": true, "dosen": true, "ukm": true, "ormawa": true}
	if roleFilter != "" && validRoles[roleFilter] {
		dbQuery = dbQuery.Where("users.role = ?", roleFilter)
	}

	// Apply search query (case-insensitive by name or email)
	if query != "" {
		searchPattern := "%" + strings.ToLower(query) + "%"
		dbQuery = dbQuery.
			Joins("LEFT JOIN mahasiswa ON mahasiswa.user_id = users.id").
			Joins("LEFT JOIN dosen ON dosen.user_id = users.id").
			Joins("LEFT JOIN ukm ON ukm.user_id = users.id").
			Joins("LEFT JOIN ormawa ON ormawa.user_id = users.id").
			Where(`LOWER(users.email) LIKE ? OR 
				LOWER(mahasiswa.name) LIKE ? OR 
				LOWER(dosen.name) LIKE ? OR 
				LOWER(ukm.name) LIKE ? OR LOWER(ukm.username) LIKE ? OR
				LOWER(ormawa.name) LIKE ? OR LOWER(ormawa.username) LIKE ?`,
				searchPattern, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern)
	}

	// Fetch results
	var users []models.User
	err := dbQuery.
		Preload("Mahasiswa").
		Preload("Dosen").
		Preload("Ukm").
		Preload("Ormawa").
		Order("users.created_at DESC").
		Limit(30).
		Find(&users).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to search users: " + err.Error(),
		})
		return
	}

	// Map to response
	var response []models.UserResponse
	for _, user := range users {
		response = append(response, models.MapUserToResponse(user))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"count":   len(response),
	})
}

// GetOnlineUsers - Get list of currently connected users
func (cc *ChatController) GetOnlineUsers(c *gin.Context) {
	_, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	onlineUserIDs := cc.hub.GetConnectedUsers()

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"online_ids": onlineUserIDs,
		"count":      len(onlineUserIDs),
	})
}

// MarkMessagesAsRead - Mark messages as read
func (cc *ChatController) MarkMessagesAsRead(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	conversationID, err := strconv.Atoi(c.Param("conversation_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid conversation ID",
		})
		return
	}

	// Check if user is participant
	var participant models.ConversationParticipant
	if err := cc.db.Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		First(&participant).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "You are not a participant in this conversation",
		})
		return
	}

	// Mark messages as read
	if err := cc.db.Model(&models.Message{}).
		Where("conversation_id = ? AND sender_id != ? AND is_read = false", conversationID, userID).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": time.Now(),
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to mark messages as read",
		})
		return
	}

	// Update participant's last read
	cc.db.Model(&models.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Update("last_read_at", time.Now())

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Messages marked as read",
	})
}

// PinConversation - Pin a conversation
func (cc *ChatController) PinConversation(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	conversationID, err := strconv.Atoi(c.Param("conversation_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid conversation ID",
		})
		return
	}

	// Check if conversation exists and user is participant
	var participant models.ConversationParticipant
	if err := cc.db.Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		First(&participant).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "You are not a participant in this conversation",
		})
		return
	}

	// Check if already pinned
	var existingPin models.PinnedConversation
	if err := cc.db.Where("user_id = ? AND conversation_id = ?", userID, conversationID).
		First(&existingPin).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "Conversation already pinned",
		})
		return
	}

	// Create pin
	pin := models.PinnedConversation{
		UserID:         userID,
		ConversationID: conversationID,
	}

	if err := cc.db.Create(&pin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to pin conversation",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Conversation pinned successfully",
	})
}

// UnpinConversation - Unpin a conversation
func (cc *ChatController) UnpinConversation(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	conversationID, err := strconv.Atoi(c.Param("conversation_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid conversation ID",
		})
		return
	}

	// Delete pin
	if err := cc.db.Where("user_id = ? AND conversation_id = ?", userID, conversationID).
		Delete(&models.PinnedConversation{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to unpin conversation",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Conversation unpinned successfully",
	})
}

// GetChatStats - Get chat statistics
func (cc *ChatController) GetChatStats(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		return
	}

	// Total conversations
	var totalConversations int64
	cc.db.Model(&models.ConversationParticipant{}).
		Where("user_id = ?", userID).
		Count(&totalConversations)

	// Total messages
	var totalMessages int64
	cc.db.Model(&models.Message{}).
		Joins("JOIN conversation_participants ON conversation_participants.conversation_id = messages.conversation_id").
		Where("conversation_participants.user_id = ?", userID).
		Where("messages.deleted_at IS NULL").
		Count(&totalMessages)

	// Unread messages
	var unreadMessages int64
	cc.db.Model(&models.Message{}).
		Joins("JOIN conversation_participants ON conversation_participants.conversation_id = messages.conversation_id").
		Where("conversation_participants.user_id = ?", userID).
		Where("messages.sender_id != ?", userID).
		Where("messages.is_read = false").
		Where("messages.deleted_at IS NULL").
		Count(&unreadMessages)

	// Group chats
	var groupChats int64
	cc.db.Model(&models.ConversationParticipant{}).
		Joins("JOIN conversations ON conversations.id = conversation_participants.conversation_id").
		Where("conversation_participants.user_id = ?", userID).
		Where("conversations.type = 'group'").
		Count(&groupChats)

	privateChats := totalConversations - groupChats

	stats := models.ChatStatsResponse{
		TotalConversations: int(totalConversations),
		TotalMessages:      int(totalMessages),
		UnreadMessages:     int(unreadMessages),
		GroupChats:         int(groupChats),
		PrivateChats:       int(privateChats),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// ==================== HELPER FUNCTIONS ====================

func (cc *ChatController) markMessagesAsRead(conversationID, userID int) {
	// Mark messages as read
	cc.db.Model(&models.Message{}).
		Where("conversation_id = ? AND sender_id != ? AND is_read = false", conversationID, userID).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": time.Now(),
		})

	// Update participant's last read
	cc.db.Model(&models.ConversationParticipant{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Update("last_read_at", time.Now())
}

func (cc *ChatController) getConversationName(convType string, name *string, creatorID int, participants []int) string {
	if name != nil && *name != "" {
		return *name
	}

	if convType == "private" && len(participants) == 1 {
		// Get other user's name
		var otherUser models.User
		cc.db.First(&otherUser, participants[0])
		return otherUser.Name
	}

	// For group chats, get names of all participants
	var userNames []string
	allParticipants := append([]int{creatorID}, participants...)

	var users []models.User
	cc.db.Where("id IN (?)", allParticipants).Find(&users)

	for _, user := range users {
		userNames = append(userNames, user.Name)
	}

	if len(userNames) > 3 {
		return fmt.Sprintf("%s, %s, %s and %d more", userNames[0], userNames[1], userNames[2], len(userNames)-3)
	}

	return strings.Join(userNames, ", ")
}
