import React, { useState, useEffect, useRef, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useParams, useNavigate } from 'react-router-dom'
import api from '../services/api'
import useAuth from '../hooks/useAuth'
import Sidebar from './Sidebar'

const _MOTION = motion

const ROLES = [
  { value: '', label: 'Semua' },
  { value: 'mahasiswa', label: 'Mahasiswa' },
  { value: 'dosen', label: 'Dosen' },
  { value: 'admin', label: 'Admin' },
  { value: 'ukm', label: 'UKM' },
  { value: 'ormawa', label: 'Ormawa' },
]

const ChatPage = ({ role }) => {
  const { user } = useAuth()
  const navigate = useNavigate()
  const { conversationId } = useParams()
  const basePath = `/${role}/pesan`

  const [conversations, setConversations] = useState([])
  const [selectedConversation, setSelectedConversation] = useState(null)
  const [messages, setMessages] = useState([])
  const [newMessage, setNewMessage] = useState('')
  const [loading, setLoading] = useState(true)
  const [sending, _setSending] = useState(false)
  const [typingUsers, setTypingUsers] = useState({})
  const [isWsConnected, setIsWsConnected] = useState(false)
  const [onlineUsers, setOnlineUsers] = useState(new Set())
  const [showMobileList, setShowMobileList] = useState(true)

  // New Chat Modal state
  const [showNewChat, setShowNewChat] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [listQuery, setListQuery] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [searchResults, setSearchResults] = useState([])
  const [searching, setSearching] = useState(false)
  const [startingChat, setStartingChat] = useState(null)
  
  // Hidden chats (local swipe-to-delete)
  const [hiddenChats, setHiddenChats] = useState(() => {
    try { return JSON.parse(localStorage.getItem('hiddenChats')) || {} } 
    catch { return {} }
  })

  const messagesEndRef = useRef(null)
  const typingTimeoutRef = useRef(null)
  const searchTimeoutRef = useRef(null)
  const selectedConvRef = useRef(null)

  // Keep ref in sync with state
  useEffect(() => {
    selectedConvRef.current = selectedConversation
  }, [selectedConversation])

  // Load conversations & connect WS
  useEffect(() => {
    if (!user) return
    loadConversations()
    api.webSocket.connect()
    api.webSocket.onMessage(handleWsMessage)
    api.webSocket.onConnectionChange(setIsWsConnected)
    // Load online users
    api.getOnlineUsers().then(r => {
      if (r.data?.online_ids) setOnlineUsers(new Set(r.data.online_ids))
    }).catch(() => {})
    return () => {
      api.webSocket.disconnect()
      api.webSocket.removeMessageCallback(handleWsMessage)
    }
  }, [user])

  // Select conversation from URL
  useEffect(() => {
    if (conversationId && conversations.length) {
      const targetId = parseInt(conversationId)
      if (selectedConvRef.current?.id !== targetId) {
        const conv = conversations.find(c => c.id === targetId)
        if (conv) {
          // Jika percakapan masih disembunyikan, jangan tampilkan di panel kanan
          const isStillHidden = hiddenChats[conv.id] === (conv.last_message?.id || 'none')
          if (isStillHidden) {
            setSelectedConversation(null)
            setMessages([])
            setShowMobileList(true)
            navigate(basePath, { replace: true })
            return
          }
          selectConversation(conv)
        } else {
          // Jika tidak ada, kembalikan ke basePath
          navigate(basePath, { replace: true })
        }
      }
    }
  }, [conversationId, conversations, hiddenChats])

  useEffect(() => { messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages])

  // Search users debounced
  useEffect(() => {
    if (!showNewChat) return
    // Don't search if query is empty and no role filter
    if (!searchQuery.trim() && !roleFilter) {
      setSearchResults([])
      return
    }
    if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current)
    searchTimeoutRef.current = setTimeout(() => {
      doSearch()
    }, 350)
    return () => clearTimeout(searchTimeoutRef.current)
  }, [searchQuery, roleFilter, showNewChat])

  const doSearch = async () => {
    setSearching(true)
    try {
      const r = await api.searchUsers(searchQuery, roleFilter)
      if (r.data?.success) setSearchResults(r.data.data || [])
    } catch (err) {
      console.error('Search users error:', err)
      setSearchResults([])
    }
    setSearching(false)
  }

  const loadConversations = async () => {
    try {
      const r = await api.getConversations()
      if (r.data?.success) {
        const convs = r.data.data || []
        setConversations(convs)
        if (!selectedConvRef.current && !conversationId) {
          const firstVisible = convs.find(c => hiddenChats[c.id] !== (c.last_message?.id || 'none') && c.last_message)
          if (firstVisible) {
            selectConversation(firstVisible)
          } else {
            setSelectedConversation(null)
            setMessages([])
          }
        }
      }
    } catch (e) { console.error('Load conversations:', e) }
    setLoading(false)
  }

  const selectConversation = async (conv) => {
    setSelectedConversation(conv)
    setMessages([])
    setShowMobileList(false)
    navigate(`${basePath}/${conv.id}`, { replace: true })
    try {
      const r = await api.getMessages(conv.id)
      if (r.data?.success) setMessages(r.data.data || [])
    } catch (e) { console.error('Load messages:', e) }
    api.markMessagesAsRead(conv.id).catch(() => {})
    setConversations(prev => prev.map(c => c.id === conv.id ? { ...c, unread_count: 0 } : c))
  }

  const sendMessage = async (e) => {
    e.preventDefault()
    if (!newMessage.trim() || !selectedConversation || sending) return
    
    const messageContent = newMessage.trim()
    setNewMessage('')
    
    // Optimistic Update
    const optimisticMsg = {
      id: `opt-${Date.now()}`,
      sender: { id: user.id, name: user.name },
      content: messageContent,
      message_type: 'text',
      created_at: new Date().toISOString(),
      is_read: false,
      status: 'sending'
    }
    
    setMessages(prev => [...prev, optimisticMsg])
    
    try {
      api.webSocket.sendTypingIndicator(selectedConversation.id, false)
      const r = await api.sendMessage(selectedConversation.id, { 
        conversation_id: selectedConversation.id, 
        content: messageContent, 
        message_type: 'text' 
      })
      
      if (r.data?.success) {
        // Replace optimistic message with actual one
        setMessages(prev => prev.map(m => m.id === optimisticMsg.id ? { ...r.data.data, status: 'sent' } : m))
      }
    } catch (e) { 
      console.error('Send:', e)
      // Mark as failed
      setMessages(prev => prev.map(m => m.id === optimisticMsg.id ? { ...m, status: 'error' } : m))
    }
  }

  const handleDeleteMessage = async (msgId) => {
    if (!window.confirm('Hapus pesan ini?')) return
    try {
      await api.deleteMessage(msgId)
      setMessages(prev => prev.filter(m => m.id !== msgId))
    } catch (e) {
      console.error('Delete message:', e)
    }
  }

  const handleHideConversation = (e, conv) => {
    e.stopPropagation()
    const newHidden = { ...hiddenChats, [conv.id]: conv.last_message?.id || 'none' }
    setHiddenChats(newHidden)
    localStorage.setItem('hiddenChats', JSON.stringify(newHidden))
    if (selectedConversation?.id === conv.id) {
      setSelectedConversation(null)
      navigate(basePath)
      setShowMobileList(true)
    }
  }

  const handleInputChange = (e) => {
    setNewMessage(e.target.value)
    if (selectedConversation && e.target.value.length > 0) {
      api.webSocket.sendTypingIndicator(selectedConversation.id, true)
      if (typingTimeoutRef.current) clearTimeout(typingTimeoutRef.current)
      typingTimeoutRef.current = setTimeout(() => {
        api.webSocket.sendTypingIndicator(selectedConversation.id, false)
      }, 2000)
    }
  }

  const handleWsMessage = useCallback((msg) => {
    const currentConv = selectedConvRef.current
    switch (msg.type) {
      case 'new_message':
        if (msg.data.conversation_id === currentConv?.id) {
          setMessages(prev => {
            // Avoid duplicates if optimistic update already added it
            const exists = prev.some(m => m.id === msg.data.message.id || (m.status === 'sending' && m.content === msg.data.message.content))
            if (exists) {
              return prev.map(m => (m.status === 'sending' && m.content === msg.data.message.content) ? msg.data.message : m)
            }
            return [...prev, msg.data.message]
          })
        }
        setConversations(prev => {
          const idx = prev.findIndex(c => c.id === msg.data.conversation_id)
          if (idx === -1) return prev
          
          const newConvs = [...prev]
          const conv = { ...newConvs[idx] }
          conv.last_message = msg.data.message
          conv.unread_count = conv.id === currentConv?.id ? 0 : (conv.unread_count || 0) + 1
          conv.updated_at = new Date().toISOString()
          
          // Move to top
          newConvs.splice(idx, 1)
          return [conv, ...newConvs]
        })
        break
      case 'message_read':
        if (msg.data.conversation_id === currentConv?.id)
          setMessages(prev => prev.map(m => m.id === msg.data.message_id ? { ...m, is_read: true } : m))
        break
      case 'message_deleted':
        if (msg.data.conversation_id === currentConv?.id) {
          setMessages(prev => prev.filter(m => m.id !== msg.data.message_id))
        }
        setConversations(prev => prev.map(c => {
          if (c.id === msg.data.conversation_id && c.last_message?.id === msg.data.message_id) {
            return { ...c, last_message: { ...c.last_message, content: '🚫 Pesan ini telah dihapus', is_deleted: true } }
          }
          return c
        }))
        break
      case 'new_conversation':
        loadConversations()
        break
      case 'typing':
        if (msg.data?.conversation_id === currentConv?.id) {
          setTypingUsers(prev => ({ ...prev, [msg.data.user_id]: { name: msg.data.user_name, isTyping: msg.data.is_typing, ts: Date.now() } }))
        }
        break
      case 'user_online':
        setOnlineUsers(prev => new Set([...prev, msg.data.user_id]))
        break
      case 'user_offline':
        setOnlineUsers(prev => { const s = new Set(prev); s.delete(msg.data.user_id); return s })
        break
    }
  }, []) // No dependencies — uses ref instead

  const startNewChat = async (contactId) => {
    if (startingChat) return
    setStartingChat(contactId)
    try {
      const r = await api.createConversation({ type: 'private', participants: [contactId] })
      if (r.data?.success) {
        setShowNewChat(false)
        setSearchQuery('')
        setRoleFilter('')
        await loadConversations()
        const convId = r.data.data
        if (convId) navigate(`${basePath}/${convId}`)
      }
    } catch (e) { console.error('Start chat:', e) }
    setStartingChat(null)
  }

  const formatTime = (d) => new Date(d).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' })
  const formatDate = (d) => {
    const date = new Date(d), today = new Date()
    if (date.toDateString() === today.toDateString()) return 'Hari ini'
    const y = new Date(today); y.setDate(y.getDate() - 1)
    if (date.toDateString() === y.toDateString()) return 'Kemarin'
    return date.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' })
  }

  const getConvName = (conv) => {
    if (conv.type === 'private' && conv.participants) {
      const other = conv.participants.find(p => p.user_id !== user?.id)
      return other?.user?.name || conv.name
    }
    return conv.name
  }

  const getTypingText = () => {
    const t = Object.values(typingUsers).filter(x => x.isTyping && Date.now() - x.ts < 3000)
    if (!t.length) return null
    return t.length === 1 ? `${t[0].name} sedang mengetik...` : `${t.length} orang sedang mengetik...`
  }

  const getRoleBadge = (r) => {
    const m = { dosen: 'bg-amber-100 text-amber-800', admin: 'bg-red-100 text-red-800', mahasiswa: 'bg-blue-100 text-blue-800', ukm: 'bg-green-100 text-green-800', ormawa: 'bg-purple-100 text-purple-800' }
    return m[r] || 'bg-gray-100 text-gray-800'
  }

  if (loading && !conversations.length) {
    return (
      <div className="flex">
        <Sidebar role={role} />
        <div className="main-content w-full flex items-center justify-center h-screen">
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-lp-accent mx-auto" />
            <p className="mt-4 text-lp-text2">Memuat percakapan...</p>
          </div>
        </div>
      </div>
    )
  }

  const visibleConversations = conversations.filter(c => hiddenChats[c.id] !== (c.last_message?.id || 'none') && c.last_message)
  const normalizedListQuery = listQuery.trim().toLowerCase()
  const filteredConversations = normalizedListQuery
    ? visibleConversations.filter((conv) => {
        const convName = (getConvName(conv) || '').toLowerCase()
        const lastContent = (conv.last_message?.content || '').toLowerCase()
        return convName.includes(normalizedListQuery) || lastContent.includes(normalizedListQuery)
      })
    : visibleConversations
  const unreadTotal = visibleConversations.reduce((acc, conv) => acc + (conv.unread_count || 0), 0)

  return (
    <div className="flex">
      <Sidebar role={role} />
      <div className="main-content w-full bg-gradient-to-br from-[#F7F9FF] via-[#FCFDFF] to-[#F1F5FF]">
        <div className="flex h-screen overflow-hidden">
          {/* Sidebar - Conversation List */}
          <div className={`${showMobileList ? 'flex' : 'hidden'} md:flex flex-col w-full md:w-[360px] lg:w-[390px] border-r border-white/70 bg-white/70 backdrop-blur-xl flex-shrink-0 z-30 shadow-[0_10px_40px_rgba(12,30,90,0.06)]`}>
            {/* Header */}
            <div className="pt-7 pb-3 px-5 sticky top-0 bg-white/70 backdrop-blur-md z-40 border-b border-white/80">
              <div className="flex items-center justify-between mb-5">
                <h2 className="text-[32px] font-extrabold text-gray-900 tracking-tight leading-none">Messages</h2>
                <button onClick={() => setShowNewChat(true)} className="w-10 h-10 flex items-center justify-center bg-white hover:bg-[#F2F6FF] text-[#007AFF] rounded-full transition-all active:scale-95 border border-[#DCE6FF] shadow-sm">
                  <i className="far fa-edit text-lg" />
                </button>
              </div>
              <div className="relative group">
                <input 
                  type="text" 
                  value={listQuery}
                  onChange={(e) => setListQuery(e.target.value)}
                  placeholder="Cari pesan atau orang..." 
                  className="w-full pl-10 pr-4 py-2.5 bg-white border border-[#E4EBFF] rounded-2xl text-[14px] focus:outline-none focus:ring-2 focus:ring-[#007AFF]/20 transition-all placeholder-gray-500 text-black shadow-[0_6px_18px_rgba(20,64,140,0.05)]" 
                />
                <i className="fas fa-search absolute left-3.5 top-1/2 -translate-y-1/2 text-gray-400 text-sm group-focus-within:text-[#007AFF] transition-colors" />
              </div>
              <div className="mt-3 flex items-center gap-2">
                <span className="px-2.5 py-1 rounded-full text-[11px] font-semibold bg-[#EDF3FF] text-[#2D5FB6] border border-[#D8E6FF]">
                  {visibleConversations.length} chat
                </span>
                <span className="px-2.5 py-1 rounded-full text-[11px] font-semibold bg-white text-[#5A6C94] border border-[#E3EAFF]">
                  {unreadTotal} belum dibaca
                </span>
              </div>
            </div>

            {/* Conversation List */}
            <div className="flex-1 overflow-x-hidden overflow-y-auto">
              {!filteredConversations.length ? (
                <div className="p-8 text-center text-lp-text3">
                  <i className="fas fa-comments text-4xl mb-3 text-gray-300 block" />
                  <p>{normalizedListQuery ? 'Percakapan tidak ditemukan' : 'Belum ada percakapan'}</p>
                  {!normalizedListQuery && (
                    <button onClick={() => setShowNewChat(true)} className="mt-3 text-lp-accent hover:underline text-sm">Mulai percakapan baru</button>
                  )}
                </div>
              ) : (
                <AnimatePresence>
                  {filteredConversations.map(conv => (
                    <motion.div 
                      key={conv.id}
                      layout
                      initial={{ opacity: 0, x: -20 }}
                      animate={{ opacity: 1, x: 0 }}
                      exit={{ opacity: 0, x: -20 }}
                      className="relative px-2 py-1.5 overflow-hidden"
                    >
                      {/* Background Delete Button (iOS style) */}
                      <div className="absolute inset-y-1.5 right-2 w-24 bg-red-500 flex items-center justify-center rounded-r-2xl">
                        <button 
                          onClick={(e) => handleHideConversation(e, conv)} 
                          className="w-full h-full text-white flex flex-col items-center justify-center active:bg-red-600 transition-colors rounded-r-2xl"
                        >
                          <i className="fas fa-trash-alt text-lg" />
                          <span className="text-[10px] font-bold mt-1 uppercase tracking-wider">Hapus</span>
                        </button>
                      </div>
                      {/* Draggable Foreground */}
                      <motion.div 
                        drag="x"
                        dragConstraints={{ left: -96, right: 0 }}
                        dragSnapToOrigin={false}
                        className={`group px-4 py-3.5 cursor-pointer relative z-10 transition-all duration-300 rounded-2xl border shadow-sm ${
                          selectedConversation?.id === conv.id
                            ? 'bg-[#EEF4FF] border-[#CFE0FF] shadow-[0_12px_28px_rgba(35,83,180,0.12)]'
                            : 'bg-white border-[#EEF2FF] hover:bg-[#F8FAFF] active:bg-[#F1F5FF]'
                        }`} 
                        onClick={() => selectConversation(conv)}
                      >
                        <div className="flex items-center gap-4">
                          <div className="relative flex-shrink-0">
                            <div className={`w-14 h-14 rounded-full flex items-center justify-center border-2 transition-transform duration-300 group-hover:scale-105 ${selectedConversation?.id === conv.id ? 'border-[#007AFF]/30 bg-[#007AFF]/10' : 'border-gray-100 bg-gray-50'}`}>
                              {conv.type === 'group' ? (
                                <i className="fas fa-users text-[#007AFF] text-xl" />
                              ) : (
                                <span className="font-bold text-[#007AFF] text-xl">{getConvName(conv)?.[0]?.toUpperCase() || '?'}</span>
                              )}
                            </div>
                            {conv.type === 'private' && conv.participants && (() => {
                              const other = conv.participants.find(p => p.user_id !== user?.id)
                              return other && onlineUsers.has(other.user_id) ? (
                                <div className="absolute bottom-0.5 right-0.5 w-4 h-4 bg-green-500 rounded-full border-2 border-white shadow-sm ring-1 ring-black/5" />
                              ) : null
                            })()}
                          </div>
                          <div className="flex-1 min-w-0 py-0.5">
                            <div className="flex justify-between items-baseline mb-1">
                              <h3 className={`font-bold truncate transition-colors ${selectedConversation?.id === conv.id ? 'text-[#007AFF]' : 'text-gray-900'}`}>{getConvName(conv)}</h3>
                              {conv.last_message && <span className="text-[11px] font-medium text-gray-400 uppercase tracking-tighter ml-2">{formatTime(conv.last_message.created_at)}</span>}
                            </div>
                            <div className="flex justify-between items-center">
                              <p className={`text-[13px] truncate pr-2 ${conv.unread_count > 0 ? 'text-gray-900 font-semibold' : 'text-gray-500'}`}>
                                {conv.last_message ? (
                                  <>
                                    <span className="opacity-70">{conv.last_message.sender?.id === user?.id ? 'Anda: ' : ''}</span>
                                    {conv.last_message.content}
                                  </>
                                ) : (
                                  <span className="italic opacity-40">Belum ada pesan</span>
                                )}
                              </p>
                              {conv.unread_count > 0 && (
                                <motion.span 
                                  initial={{ scale: 0 }}
                                  animate={{ scale: 1 }}
                                  className="bg-[#007AFF] text-white text-[10px] rounded-full h-5 min-w-[20px] px-1.5 flex items-center justify-center font-bold shadow-lg shadow-[#007AFF]/20"
                                >
                                  {conv.unread_count}
                                </motion.span>
                              )}
                            </div>
                          </div>
                        </div>
                      </motion.div>
                    </motion.div>
                  ))}
                </AnimatePresence>
              )}
            </div>
          </div>

          {/* Main Chat Area */}
          <div className={`${!showMobileList ? 'flex' : 'hidden'} md:flex flex-col flex-1 bg-transparent`}>
            {selectedConversation ? (
              <>
                {/* Chat Header (iMessage style) */}
                <div className="px-6 py-4 border-b border-white/80 bg-white/75 backdrop-blur-2xl sticky top-0 z-40 flex items-center justify-between shadow-[0_10px_30px_rgba(30,64,130,0.08)]">
                  <div className="flex items-center gap-3">
                    <button onClick={() => setShowMobileList(true)} className="md:hidden w-10 h-10 -ml-2 text-[#007AFF] flex items-center justify-center hover:bg-blue-50 rounded-full transition-colors">
                      <i className="fas fa-chevron-left text-xl" />
                    </button>
                    <div className="flex items-center gap-3">
                      <div className="w-10 h-10 rounded-full bg-gradient-to-tr from-[#EAF1FF] to-white border border-[#DCE7FF] flex items-center justify-center overflow-hidden shadow-sm">
                        {selectedConversation.type === 'group' ? <i className="fas fa-users text-[#007AFF]" /> : <span className="font-bold text-[#007AFF] text-lg">{getConvName(selectedConversation)?.[0]?.toUpperCase() || '?'}</span>}
                      </div>
                      <div className="flex flex-col">
                        <h2 className="font-bold text-gray-900 text-[16px] leading-tight">{getConvName(selectedConversation)}</h2>
                        <div className="flex items-center gap-1.5 mt-0.5">
                          <div className={`w-2 h-2 rounded-full ${isWsConnected ? 'bg-green-500 animate-pulse' : 'bg-red-400'}`} />
                          <span className="text-[11px] font-medium text-gray-500 uppercase tracking-widest">{isWsConnected ? 'Online' : 'Menghubungkan...'}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <button className="w-10 h-10 flex items-center justify-center text-gray-400 hover:text-[#007AFF] hover:bg-blue-50 rounded-full transition-all">
                      <i className="fas fa-phone-alt" />
                    </button>
                    <button className="w-10 h-10 flex items-center justify-center text-gray-400 hover:text-[#007AFF] hover:bg-blue-50 rounded-full transition-all">
                      <i className="fas fa-video" />
                    </button>
                    <button className="w-10 h-10 flex items-center justify-center text-gray-400 hover:text-[#007AFF] hover:bg-blue-50 rounded-full transition-all">
                      <i className="fas fa-info-circle" />
                    </button>
                  </div>
                </div>

                {/* Messages */}
                <div className="flex-1 overflow-y-auto px-4 md:px-6 py-6 bg-gradient-to-b from-[#F8FAFF] via-[#FAFCFF] to-[#F3F7FF] custom-scrollbar">
                  <div className="max-w-4xl mx-auto w-full space-y-2">
                    {messages.map((msg, i) => {
                    const showDate = i === 0 || formatDate(messages[i-1].created_at) !== formatDate(msg.created_at)
                    const isMine = msg.sender?.id === user?.id
                    const isSequential = i > 0 && messages[i-1].sender?.id === msg.sender?.id && !showDate
                    const isLastInSequence = i === messages.length - 1 || messages[i+1]?.sender?.id !== msg.sender?.id || (formatDate(messages[i+1].created_at) !== formatDate(msg.created_at))
                    
                    return (
                      <React.Fragment key={msg.id}>
                        {showDate && (
                          <div className="flex items-center justify-center my-6">
                            <div className="h-px bg-[#DCE5FF] flex-1" />
                            <span className="px-3 py-1 text-[#6D7EA8] text-[10px] font-bold uppercase tracking-widest bg-white/90 border border-[#E1E9FF] rounded-full shadow-sm">{formatDate(msg.created_at)}</span>
                            <div className="h-px bg-[#DCE5FF] flex-1" />
                          </div>
                        )}
                        
                        <motion.div 
                          initial={{ opacity: 0, y: 10, scale: 0.95 }}
                          animate={{ opacity: 1, y: 0, scale: 1 }}
                          transition={{ duration: 0.2 }}
                          className={`flex ${isMine ? 'justify-end' : 'justify-start'} group items-end gap-1.5`}
                        >
                          {isMine && !msg.id.toString().startsWith('opt-') && (
                            <button 
                              onClick={() => handleDeleteMessage(msg.id)} 
                              className="opacity-0 group-hover:opacity-100 w-8 h-8 flex items-center justify-center text-gray-400 hover:text-red-500 transition-all active:scale-90"
                            >
                              <i className="fas fa-trash-alt text-xs" />
                            </button>
                          )}
                          
                          <div className={`flex flex-col ${isMine ? 'items-end' : 'items-start'} max-w-[80%]`}>
                            {!isMine && !isSequential && selectedConversation.type === 'group' && (
                              <div className="text-[11px] font-bold text-gray-400 ml-3 mb-1 uppercase tracking-tight">{msg.sender?.name}</div>
                            )}
                            <div className={`relative px-4 py-2.5 text-[15px] leading-relaxed shadow-sm transition-all ${
                              isMine 
                                ? `bg-gradient-to-br from-[#0A84FF] to-[#0071F3] text-white ${isLastInSequence ? 'rounded-2xl rounded-br-sm' : 'rounded-2xl'} ${msg.status === 'sending' ? 'opacity-70' : ''} shadow-[0_10px_22px_rgba(0,113,243,0.28)]` 
                                : `bg-white text-[#101828] border border-[#E6ECFF] ${isLastInSequence ? 'rounded-2xl rounded-bl-sm' : 'rounded-2xl'} shadow-[0_8px_18px_rgba(16,24,40,0.06)]`
                            }`}>
                              {msg.message_type === 'image' ? (
                                <img src={msg.file_url} alt="" className="max-w-full rounded-xl shadow-md" />
                              ) : (
                                <div className="whitespace-pre-wrap">{msg.content}</div>
                              )}
                              
                              {/* Status Indicators */}
                              {isMine && msg.status === 'sending' && (
                                <div className="absolute -left-5 bottom-1">
                                  <div className="w-3 h-3 border-2 border-[#007AFF]/30 border-t-[#007AFF] rounded-full animate-spin" />
                                </div>
                              )}
                            </div>
                            
                            {isMine && isLastInSequence && (
                              <div className="flex items-center gap-1 mt-1 mr-1">
                                <span className="text-[10px] font-bold text-gray-400 uppercase tracking-tighter">
                                  {msg.is_read ? 'Dibaca' : (msg.status === 'sending' ? 'Mengirim' : formatTime(msg.created_at))}
                                </span>
                                {msg.is_read && <i className="fas fa-check-double text-[8px] text-[#007AFF]" />}
                              </div>
                            )}
                          </div>
                        </motion.div>
                      </React.Fragment>
                    )
                  })}
                    {getTypingText() && (
                      <motion.div initial={{ opacity: 0, y: 5 }} animate={{ opacity: 1, y: 0 }} className="flex justify-start items-end gap-2 mt-2 ml-1">
                        <div className="bg-white border border-[#E2E9FF] rounded-2xl rounded-bl-sm px-4 py-3 flex items-center gap-2 shadow-sm">
                          {[0,0.15,0.3].map((d,i) => <motion.div key={i} animate={{ y: [0, -4, 0] }} transition={{ repeat: Infinity, duration: 0.6, delay: d }} className="w-1.5 h-1.5 bg-[#7F90B8] rounded-full" />)}
                          <span className="text-[11px] text-[#7A8CB5] font-medium">{getTypingText()}</span>
                        </div>
                      </motion.div>
                    )}
                    <div ref={messagesEndRef} />
                  </div>
                </div>

                {/* Input (iMessage style) */}
                <form onSubmit={sendMessage} className="px-4 md:px-6 py-4 bg-white/70 backdrop-blur-xl border-t border-white/80 z-40">
                  <div className="max-w-4xl mx-auto">
                    <div className="flex items-end gap-2.5 bg-white border border-[#DCE6FF] rounded-[26px] pl-3.5 pr-2 py-2 focus-within:border-[#7BA9FF] focus-within:shadow-[0_10px_25px_rgba(0,122,255,0.14)] transition-all">
                    <button type="button" className="w-9 h-9 flex items-center justify-center text-[#007AFF] hover:bg-blue-50 rounded-full transition-colors">
                      <i className="fas fa-plus" />
                    </button>
                    <button type="button" className="w-9 h-9 flex items-center justify-center text-gray-400 hover:text-[#007AFF] hover:bg-blue-50 rounded-full transition-colors">
                      <i className="far fa-smile" />
                    </button>
                    <textarea
                      value={newMessage} onChange={handleInputChange}
                      onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(e) } }}
                      placeholder="Ketik pesan..."
                      className="flex-1 py-1.5 text-[15px] resize-none focus:outline-none bg-transparent max-h-32 self-center text-gray-800 placeholder:text-gray-400"
                      rows="1"
                    />
                    <button type="submit" disabled={!newMessage.trim() && !sending}
                      className={`w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0 transition-all ${newMessage.trim() ? 'bg-gradient-to-br from-[#0A84FF] to-[#0071F3] text-white scale-100 shadow-[0_8px_18px_rgba(0,113,243,0.35)]' : 'bg-[#EEF2FF] text-gray-400 scale-95'}`}>
                      {sending ? <i className="fas fa-spinner fa-spin text-sm" /> : <i className="fas fa-arrow-up text-sm" />}
                    </button>
                    </div>
                  </div>
                </form>
              </>
            ) : (
              <div className="flex-1 flex flex-col items-center justify-center p-8 bg-gradient-to-b from-[#F8FAFF] to-[#F1F6FF]">
                <div className="w-16 h-16 bg-white shadow-[0_10px_22px_rgba(41,78,153,0.14)] rounded-full flex items-center justify-center mb-4 border border-[#E2EBFF]">
                  <i className="fas fa-comment-dots text-2xl text-[#7E90B8]" />
                </div>
                <h3 className="text-[17px] font-semibold text-[#62749C] mb-1">Mulai Obrolan</h3>
                <p className="text-[13px] text-[#90A0C3] text-center max-w-md">Pilih percakapan atau buat chat baru untuk mulai kirim pesan.</p>
                <button onClick={() => setShowNewChat(true)} className="mt-6 text-[#007AFF] text-[15px] font-semibold hover:underline transition-colors">
                  New Message
                </button>
              </div>
            )}
          </div>
        </div>

        {/* New Chat Modal */}
        {showNewChat && (
          <div className="fixed inset-0 bg-[#0B1220]/55 backdrop-blur-md flex items-center justify-center z-[90] p-4" onClick={() => setShowNewChat(false)}>
            <div className="bg-white/95 rounded-3xl w-full max-w-md max-h-[80vh] flex flex-col shadow-[0_30px_60px_rgba(15,23,42,0.35)] border border-[#DCE6FF]" onClick={e => e.stopPropagation()}>
              {/* Modal Header */}
              <div className="p-5 border-b border-[#E4EBFF]">
                <div className="flex justify-between items-center mb-4">
                  <h3 className="text-lg font-bold text-[#1D2B4F] tracking-tight">Chat Baru</h3>
                  <button onClick={() => setShowNewChat(false)} className="text-[#7E8CAD] hover:text-[#2F3F63] w-8 h-8 flex items-center justify-center rounded-lg hover:bg-[#EEF3FF] transition-colors"><i className="fas fa-times" /></button>
                </div>

                {/* Role Filter */}
                <div className="flex gap-1.5 mb-3 flex-wrap">
                  {ROLES.map(r => (
                    <button key={r.value} onClick={() => setRoleFilter(r.value)}
                      className={`px-3 py-1.5 rounded-full text-xs font-semibold transition-all ${roleFilter === r.value ? 'bg-gradient-to-r from-[#0A84FF] to-[#0071F3] text-white shadow-[0_8px_18px_rgba(0,113,243,0.3)]' : 'bg-white text-[#596B91] border border-[#DEE7FF] hover:border-[#8DB2FF]'}`}>
                      {r.label}
                    </button>
                  ))}
                </div>

                {/* Search Input */}
                <div className="relative">
                  <i className="fas fa-search absolute left-3 top-2.5 text-[#8C9AC0] text-sm" />
                  <input type="text" value={searchQuery} onChange={e => setSearchQuery(e.target.value)}
                    placeholder="Cari nama atau email..."
                    className="w-full pl-9 pr-4 py-2.5 bg-[#F8FAFF] border border-[#DFE8FF] rounded-xl text-sm text-[#1C2746] focus:outline-none focus:ring-2 focus:ring-[#0A84FF]/25"
                    autoFocus />
                </div>
              </div>

              {/* Search Results */}
              <div className="flex-1 overflow-y-auto p-2">
                {searching ? (
                  <div className="text-center py-8"><div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[#0A84FF] mx-auto" /><p className="mt-2 text-[#7E90B8] text-sm">Mencari...</p></div>
                ) : !searchResults.length ? (
                  <div className="text-center py-8 text-[#8394BA]">
                    <i className="fas fa-users text-3xl mb-3 text-gray-300 block" />
                    <p className="text-sm">{searchQuery ? 'Tidak ditemukan' : 'Ketik untuk mencari pengguna'}</p>
                  </div>
                ) : (
                  searchResults.map(u => (
                    <div key={u.id} className="flex items-center gap-3 p-3 hover:bg-[#F2F6FF] rounded-2xl cursor-pointer transition-colors group border border-transparent hover:border-[#DDE7FF]" onClick={() => startNewChat(u.id)}>
                      <div className="relative flex-shrink-0">
                        <div className="w-10 h-10 rounded-full bg-gradient-to-br from-[#DCE9FF] to-[#F2F7FF] flex items-center justify-center border border-[#D8E5FF]">
                          <span className="font-bold text-[#0A84FF] text-sm">{u.name?.[0]?.toUpperCase() || '?'}</span>
                        </div>
                        {onlineUsers.has(u.id) && <div className="absolute -bottom-0.5 -right-0.5 w-3 h-3 bg-green-500 rounded-full border-2 border-white" />}
                      </div>
                      <div className="flex-1 min-w-0">
                        <h4 className="font-semibold text-sm text-[#1A2646] truncate">{u.name}</h4>
                        <p className="text-xs text-[#8494B8] truncate">{u.email}</p>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${getRoleBadge(u.role)}`}>{u.role}</span>
                        {startingChat === u.id ? <i className="fas fa-spinner fa-spin text-[#0A84FF]" /> : <i className="fas fa-comment text-[#0A84FF] opacity-0 group-hover:opacity-100 transition-opacity" />}
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default ChatPage
