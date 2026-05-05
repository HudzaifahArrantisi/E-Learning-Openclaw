// src/components/Sidebar.jsx
import React, { useEffect, useMemo, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import useAuth from '../hooks/useAuth'
import { 
  FaHome, FaUser, FaBook, FaMoneyBill, FaCalendar, 
  FaComment, FaNewspaper, FaChartBar, FaCog, 
  FaTimes, FaGraduationCap, FaTasks, FaUpload,
  FaInstagram, FaBookmark, FaUsers, FaStore,
  FaSignOutAlt, FaShieldAlt
} from 'react-icons/fa'
import { IoIosSettings, IoIosPaper } from 'react-icons/io'
import { MdDashboard, MdClass, MdPayment } from 'react-icons/md'

const Sidebar = ({ role, isOpen, onClose }) => {
  const location = useLocation()
  const navigate = useNavigate()
  const { logout, user } = useAuth()
  const [internalOpen, setInternalOpen] = useState(false)

  const isControlled = useMemo(() => typeof isOpen === 'boolean', [isOpen])
  const sidebarOpen = isControlled ? isOpen : internalOpen

  useEffect(() => {
    if (isControlled) return

    const handleGlobalToggle = () => {
      setInternalOpen(prev => !prev)
    }

    window.addEventListener('nf-sidebar-toggle', handleGlobalToggle)
    return () => window.removeEventListener('nf-sidebar-toggle', handleGlobalToggle)
  }, [isControlled])

  const closeSidebar = () => {
    if (isControlled) {
      if (typeof onClose === 'function') {
        onClose()
      }
      return
    }

    setInternalOpen(false)
  }

  const shouldHideSidebar = useMemo(() => {
    const hiddenPathMatchers = [/^\/profile\//i, /\/popup(\/|$)/i]
    return hiddenPathMatchers.some((matcher) => matcher.test(location.pathname))
  }, [location.pathname])

  const handleLogout = async () => {
    try {
      await logout()
      navigate('/')
    } catch (error) {
      console.error('Logout error:', error)
    }
  }


  const isActive = (path) => location.pathname === path || location.pathname.startsWith(path + '/')

  const menuItems = {
    mahasiswa: [
      { path: '/mahasiswa', label: 'Dashboard', icon: <MdDashboard className="text-lg" /> },
      { path: '/mahasiswa/profile', label: 'Profile', icon: <FaUser className="text-lg" /> },
      { path: '/mahasiswa/matkul', label: 'My Courses', icon: <MdClass className="text-lg" /> },
      { path: '/mahasiswa/tugas', label: 'Tugas', icon: <FaTasks className="text-lg" /> },
      { path: '/mahasiswa/materi', label: 'Materi', icon: <FaBook className="text-lg" /> },
      { path: '/mahasiswa/pembayaran-ukt', label: 'Pembayaran UKT', icon: <MdPayment className="text-lg" /> },
      { path: '/mahasiswa/cari-invoice', label: 'Invoice', icon: <FaMoneyBill className="text-lg" /> },
      { path: '/mahasiswa/scan-absensi', label: 'Jadwal dan Absensi', icon: <FaCalendar className="text-lg" /> },
      { path: '/mahasiswa/transkrip-nilai', label: 'Nilai', icon: <FaChartBar className="text-lg" /> },
      { path: '/mahasiswa/pesan', label: 'Chat', icon: <FaComment className="text-lg" /> },
    ],
    dosen: [
      { path: '/dosen', label: 'Dashboard', icon: <MdDashboard className="text-lg" /> },
      { path: '/dosen/course', label: user?.email === 'superdosen@nurulfikri.ac.id' ? 'Kelola Semua Matkul' : 'Kelas Saya', icon: <FaBook className="text-lg" /> },
      { path: '/dosen/manage-matkul', label: 'Manajemen Matkul', icon: <FaShieldAlt className="text-lg" />, hidden: user?.email !== 'superdosen@nurulfikri.ac.id' },
      { path: '/dosen/absensi', label: 'Absensi', icon: <FaCalendar className="text-lg" />, hidden: user?.email === 'superdosen@nurulfikri.ac.id' },
      { path: '/dosen/pesan', label: 'Pesan', icon: <FaComment className="text-lg" />, hidden: user?.email === 'superdosen@nurulfikri.ac.id' }
    ],
    admin: [
      { path: '/admin', label: 'Dashboard', icon: <MdDashboard className="text-lg" /> },
      { path: '/admin/akun', label: 'Akun Saya', icon: <FaUser className="text-lg" /> },
      { path: '/admin/posting-pemberitahuan', label: 'Buat Posting', icon: <IoIosPaper className="text-lg" /> },
      { path: '/admin/pemantauan-ukt', label: 'Monitor UKT', icon: <FaMoneyBill className="text-lg" /> },
      { path: '/admin/pesan', label: 'Pesan', icon: <FaComment className="text-lg" /> },
      { path: '/admin/setting-profile', label: 'Pengaturan', icon: <IoIosSettings className="text-lg" /> }
    ],
    orangtua: [
      { path: '/ortu', label: 'Dashboard', icon: <MdDashboard className="text-lg" /> },
      { path: '/ortu/pantau-kehadiran', label: 'Kehadiran', icon: <FaCalendar className="text-lg" /> },
      { path: '/ortu/pembayaran-ukt', label: 'UKT', icon: <FaMoneyBill className="text-lg" /> }
    ],
    ukm: [
      { path: '/ukm', label: 'Dashboard', icon: <MdDashboard className="text-lg" /> },
      { path: '/ukm/akun', label: 'Profil', icon: <FaUser className="text-lg" /> },
      { path: '/ukm/posting', label: 'Posting', icon: <FaNewspaper className="text-lg" /> },
      { path: '/ukm/setting-profile', label: 'Pengaturan', icon: <IoIosSettings className="text-lg" /> }
    ],
    ormawa: [
      { path: '/ormawa', label: 'Dashboard', icon: <MdDashboard className="text-lg" /> },
      { path: '/ormawa/akun', label: 'Profil', icon: <FaUser className="text-lg" /> },
      { path: '/ormawa/posting', label: 'Posting', icon: <FaNewspaper className="text-lg" /> },
      { path: '/ormawa/setting-profile', label: 'Pengaturan', icon: <IoIosSettings className="text-lg" /> }
    ]
  }

  const items = (menuItems[role] || []).filter(item => !item.hidden)

  if (shouldHideSidebar) return null

  return (
    <>
      {/* Overlay untuk mobile */}
      {sidebarOpen && (
        <div 
          className="fixed inset-0 bg-black/30 backdrop-blur-sm z-[70] lg:hidden animate-fadeIn"
          onClick={closeSidebar}
        />
      )}
      
      {/* Sidebar - Glassmorphism Style */}
      <div className={`
        fixed lg:sticky top-0 left-0 z-[80]
        w-[260px] bg-white/80 backdrop-blur-2xl border-r border-lp-border
        transform transition-all duration-500 ease-in-out
        ${sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}
        flex flex-col h-[100dvh] min-h-[100dvh] overflow-hidden
      `}>

        {/* Header Sidebar */}
        <div className="px-5 pt-7 pb-5 border-b border-lp-border">
          <div className="flex items-center justify-between mb-1">
            <div className="flex items-center gap-3">
              <div className="w-9 h-9 rounded-xl bg-lp-accentS border border-lp-borderA flex items-center justify-center">
                <svg className="w-[18px] h-[18px] stroke-lp-accent fill-none stroke-2 [stroke-linecap:round] [stroke-linejoin:round]" viewBox="0 0 24 24">
                  <path d="M22 10v6M2 10l10-5 10 5-10 5-10-5z" />
                  <path d="M6 12v5c3.33 1.67 8.67 1.67 12 0v-5" />
                </svg>
              </div>
              <div>
                <h2 className="text-[13px] font-bold text-lp-text tracking-wide">StudentHub</h2>
                <p className="text-[10px] font-mono text-lp-text3 tracking-[0.08em] uppercase">{role}</p>
              </div>
            </div>
            <button 
              onClick={closeSidebar}
              className="lg:hidden p-2 hover:bg-lp-surface rounded-xl transition-colors"
            >
              <FaTimes className="text-lp-text3 text-sm" />
            </button>
          </div>
        </div>

        {/* Section Label */}
        <div className="px-5 pt-5 pb-2">
          <span className="text-[10px] font-mono font-medium tracking-[0.14em] uppercase text-lp-text3">Navigation</span>
        </div>

        {/* Menu Items */}
        <nav className="flex-1 px-3 space-y-0.5 overflow-y-auto overscroll-contain custom-scrollbar">
          {items.map((item) => (
            <Link
              key={item.path}
              to={item.path}
              onClick={() => window.innerWidth < 1024 && closeSidebar()}
              className={`
                flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all duration-200 group
                ${(isActive(item.path) || location.pathname.startsWith(item.path)) 
                  ? 'bg-lp-accentS text-lp-atext font-semibold' 
                  : 'text-lp-text2 hover:bg-lp-surface hover:text-lp-text'
                }
              `}
            >
              <div className={`text-base ${(isActive(item.path) || location.pathname.startsWith(item.path)) ? 'text-lp-atext' : 'text-lp-text3 group-hover:text-lp-text2'}`}>
                {item.icon}
              </div>
              <span className="text-[13px] font-light">{item.label}</span>
              {(isActive(item.path) || location.pathname.startsWith(item.path)) && (
                <div className="ml-auto w-1.5 h-1.5 rounded-full bg-lp-accent animate-pulse" />
              )}
            </Link>
          ))}

          {/* Logout Button */}
          <button
            onClick={handleLogout}
            className="flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all duration-200 group text-lp-text2 hover:bg-lp-surface hover:text-lp-text w-full mt-4 border-t border-lp-border pt-4"
          >
            <div className="text-base text-lp-text3 group-hover:text-red-500">
              <FaSignOutAlt className="text-lg" />
            </div>
            <span className="text-[13px] font-light">Logout</span>
          </button>
        </nav>

        {/* Footer Sidebar */}
        <div className="px-5 py-4 border-t border-lp-border pb-[max(1rem,env(safe-area-inset-bottom))]">
          <div className="flex items-center gap-1.5 justify-center">
            <div className="w-1 h-1 rounded-full bg-lp-accent/40" />
            <span className="text-[9px] font-mono text-lp-text3 tracking-[0.06em]">© 2025 Student Hub</span>
            <div className="w-1 h-1 rounded-full bg-lp-accent/40" />
          </div>
        </div>
      </div>
    </>
  )
}

export default Sidebar
