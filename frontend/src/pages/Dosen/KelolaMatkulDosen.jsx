import React, { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import Sidebar from '../../components/Sidebar'
import api from '../../services/api'
import { resolveBackendAssetUrl } from '../../utils/assetUrl'
import { 
  FiUpload, FiFileText, FiEye, FiTrash2, FiDownload, 
  FiCalendar, FiChevronLeft, FiChevronRight, FiPlus,
  FiGrid, FiList, FiClock, FiCheckCircle, FiXCircle,
  FiEdit2, FiMoreVertical, FiBookOpen, FiFile, FiFilter
} from 'react-icons/fi'
import { motion, AnimatePresence } from 'framer-motion'

const KelolaMatkulDosen = () => {
  const { courseId } = useParams()
  const [pertemuanList, setPertemuanList] = useState([])
  const [showUploadMateri, setShowUploadMateri] = useState(false)
  const [showCreateTugas, setShowCreateTugas] = useState(false)
  const [showDetailPertemuan, setShowDetailPertemuan] = useState(false)
  const [selectedPertemuan, setSelectedPertemuan] = useState(null)
  const [pertemuanDetail, setPertemuanDetail] = useState({ materi: [], tugas: [] })
  const [formData, setFormData] = useState({
    pertemuan: '',
    title: '',
    desc: '',
    due_date: '',
    file: null,
    file_tugas: null
  })
  const [loading, setLoading] = useState(true)
  const [courseName, setCourseName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [viewMode, setViewMode] = useState('grid') 

  useEffect(() => {
    fetchPertemuanList()
  }, [courseId])

  const fetchPertemuanList = async () => {
    try {
      setLoading(true)

      // Fetch course info
      const courseRes = await api.getCourseInfo(courseId)
      if (courseRes.data && courseRes.data.data) {
        setCourseName(courseRes.data.data.nama)
      }

      const response = await api.getPertemuanList(courseId, 'dosen')
      setPertemuanList(response.data.data || [])
    } catch (error) {
      console.error('Error fetching pertemuan:', error)
      alert('Gagal memuat pertemuan: ' + (error.response?.data?.message || error.message))
    } finally {
      setLoading(false)
    }
  }

  const fetchPertemuanDetail = async (pertemuan) => {
    try {
      setDetailLoading(true)
      const response = await api.getDosenPertemuanDetail(courseId, pertemuan)
      setPertemuanDetail(response.data.data)
      setSelectedPertemuan(pertemuan)
      setShowDetailPertemuan(true)
    } catch (error) {
      console.error('Error fetching pertemuan detail:', error)
      alert('Gagal memuat detail pertemuan: ' + (error.response?.data?.message || error.message))
    } finally {
      setDetailLoading(false)
    }
  }

  const handleUploadMateri = async (e) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const data = new FormData()
      data.append('course_id', courseId)
      data.append('pertemuan', formData.pertemuan)
      data.append('title', formData.title)
      data.append('desc', formData.desc)
      if (formData.file) data.append('file', formData.file)

      await api.uploadMateri(data)
      alert('Materi berhasil diupload!')
      setShowUploadMateri(false)
      setFormData(prev => ({ ...prev, title: '', desc: '', file: null }))
      fetchPertemuanList()
    } catch (error) {
      console.error('Error uploading materi:', error)
      alert('Gagal upload materi: ' + (error.response?.data?.message || error.message))
    } finally {
      setSubmitting(false)
    }
  }

  const handleCreateTugas = async (e) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const data = new FormData()
      data.append('course_id', courseId)
      data.append('pertemuan', formData.pertemuan)
      data.append('title', formData.title)
      data.append('desc', formData.desc)
      data.append('due_date', formData.due_date)
      if (formData.file_tugas) data.append('file_tugas', formData.file_tugas)

      await api.createTugas(data)
      alert('Tugas berhasil dibuat!')
      setShowCreateTugas(false)
      setFormData(prev => ({ ...prev, title: '', desc: '', due_date: '', file_tugas: null }))
      fetchPertemuanList()
    } catch (error) {
      console.error('Error creating tugas:', error)
      alert('Gagal membuat tugas: ' + (error.response?.data?.message || error.message))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDeleteMateri = async (materiId) => {
    if (!window.confirm('Apakah Anda yakin ingin menghapus materi ini?')) {
      return
    }

    try {
      await api.deleteMateri(materiId)
      alert('Materi berhasil dihapus!')
      if (selectedPertemuan) {
        fetchPertemuanDetail(selectedPertemuan)
      }
      fetchPertemuanList()
    } catch (error) {
      console.error('Error Delete Materi:', error)
      alert('Gagal menghapus materi: ' + (error.response?.data?.message || error.message))
    }
  }

  const handleDeleteTugas = async (tugasId) => {
    if (!window.confirm('Apakah Anda yakin ingin menghapus tugas ini? Semua pengumpulan mahasiswa juga akan dihapus.')) {
      return
    }

    try {
      await api.deleteTugas(tugasId)
      alert('Tugas berhasil dihapus!')
      if (selectedPertemuan) {
        fetchPertemuanDetail(selectedPertemuan)
      }
      fetchPertemuanList()
    } catch (error) {
      console.error('Error deleting tugas:', error)
      alert('Gagal menghapus tugas: ' + (error.response?.data?.message || error.message))
    }
  }

  const handleFileChange = (e, field) => {
    setFormData(prev => ({
      ...prev,
      [field]: e.target.files[0]
    }))
  }

  const toggleSidebar = () => {
    setSidebarOpen(!sidebarOpen)
  }


  if (loading) {
    return (
      <div className="flex min-h-screen bg-lp-bg">
        <Sidebar role="dosen" isOpen={sidebarOpen} onClose={toggleSidebar} />
        <div className="flex-1 lg:ml-0 transition-all duration-300 relative z-10">
          <div className="p-6 lg:p-10 max-w-7xl mx-auto">
            {/* Skeleton Header */}
            <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-8 mb-12 animate-pulse">
              <div className="space-y-4">
                <div className="h-4 w-32 bg-lp-border rounded-full"></div>
                <div className="h-12 w-96 bg-lp-border rounded-2xl"></div>
                <div className="h-4 w-64 bg-lp-border rounded-full"></div>
              </div>
              <div className="h-14 w-48 bg-lp-border rounded-full"></div>
            </div>

            {/* Skeleton Grid */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
              {[1,2,3,4,5,6,7,8].map(item => (
                <div key={item} className="h-64 bg-white border border-lp-border rounded-[2.5rem] animate-pulse"></div>
              ))}
            </div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen bg-lp-bg">
       {/* Background Decorative Layer */}
       <div className="fixed inset-0 pointer-events-none z-0 overflow-hidden">
        <div className="absolute top-[-10%] right-[-10%] w-[60%] h-[60%] bg-lp-text/5 blur-[120px] rounded-full" />
        <div className="absolute bottom-[-10%] left-[-10%] w-[50%] h-[50%] bg-lp-text/5 blur-[120px] rounded-full" />
      </div>

      <Sidebar role="dosen" isOpen={sidebarOpen} onClose={toggleSidebar} />
      
      {/* Main Content */}
      <div className="flex-1 transition-all duration-300 min-w-0 relative z-10">
        <div className="p-6 lg:p-10 max-w-7xl mx-auto">
          
          {/* Header Section */}
          <motion.div 
            initial={{ opacity: 0, y: -20 }}
            animate={{ opacity: 1, y: 0 }}
            className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-8 mb-12"
          >
            <div>
              <div className="flex items-center gap-3 mb-4">
                <button 
                  onClick={toggleSidebar}
                  className="lg:hidden p-3 rounded-xl bg-white border border-lp-border hover:bg-lp-surface transition-all"
                >
                  <FiChevronRight className="text-lp-text" />
                </button>
                <Link 
                  to="/dosen/course"
                  className="text-[11px] font-mono font-medium tracking-[0.2em] uppercase text-lp-text3 hover:text-lp-text transition-colors flex items-center gap-2"
                >
                  <FiChevronLeft /> BACK TO COURSES
                </Link>
              </div>
              
              <h1 className="text-4xl md:text-5xl font-light text-lp-text tracking-tight mb-3">
                {courseName || courseId}
                <span className="text-lp-text3 block text-lg font-normal mt-2">Course Management Suite</span>
              </h1>
              <p className="text-lp-text2 font-light max-w-xl">
                Otomasi distribusi materi dan tugas untuk setiap pertemuan kelas guna meningkatkan efektivitas pengajaran.
              </p>
            </div>
            
            <div className="flex items-center gap-4">
              <Link 
                to={`/dosen/penilaian/${courseId}`}
                className="group px-8 py-4 bg-lp-text text-lp-bg rounded-full text-[13px] font-bold hover:bg-lp-atext hover:-translate-y-1 transition-all duration-500 uppercase tracking-widest flex items-center gap-3 shadow-[0_12px_24px_rgba(0,0,0,0.1)]"
              >
                <FiEye className="text-lg" />
                <span>Monitoring Penilaian</span>
              </Link>
            </div>
          </motion.div>

          {/* Stats & Quick Actions Overview */}
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 mb-12">
            
            {/* Stats Grid */}
            <div className="lg:col-span-8 grid grid-cols-2 sm:grid-cols-4 gap-6">
               {[
                 { label: 'Sessions', val: pertemuanList.length, icon: FiGrid },
                 { label: 'Modules', val: pertemuanList.filter(p => p.has_materi).length, icon: FiFileText },
                 { label: 'Assessments', val: pertemuanList.filter(p => p.has_tugas).length, icon: FiCalendar },
                 { label: 'Active', val: pertemuanList.filter(p => p.has_materi || p.has_tugas).length, icon: FiCheckCircle }
               ].map((stat, i) => (
                 <motion.div 
                   key={i}
                   initial={{ opacity: 0, y: 20 }}
                   animate={{ opacity: 1, y: 0 }}
                   transition={{ delay: i * 0.1 }}
                   className="bg-white border border-lp-border rounded-[2rem] p-6 hover:shadow-[0_24px_48px_rgba(0,0,0,0.04)] transition-all duration-500"
                 >
                    <div className="p-3 bg-lp-surface/50 rounded-xl w-fit mb-4">
                      <stat.icon className="text-lp-text2" />
                    </div>
                    <p className="text-[11px] font-mono uppercase tracking-wider text-lp-text3 mb-1">{stat.label}</p>
                    <h3 className="text-2xl font-bold text-lp-text">{stat.val}</h3>
                 </motion.div>
               ))}
            </div>

            {/* Quick Action Block */}
            <div className="lg:col-span-4 flex flex-col gap-4">
               <motion.button
                 initial={{ opacity: 0, x: 20 }}
                 animate={{ opacity: 1, x: 0 }}
                 onClick={() => {
                   setSelectedPertemuan(null);
                   setFormData(prev => ({ ...prev, pertemuan: '' }));
                   setShowUploadMateri(true);
                 }}
                 className="flex-1 group relative overflow-hidden bg-white border border-lp-border rounded-[2rem] p-6 flex items-center justify-between hover:border-lp-text transition-all duration-500"
               >
                 <div className="flex items-center gap-4">
                   <div className="p-3 bg-lp-surface group-hover:bg-lp-text group-hover:text-white transition-all duration-500 rounded-2xl">
                     <FiUpload />
                   </div>
                   <div className="text-left">
                     <p className="font-bold text-lp-text text-[15px] tracking-tight">Upload Materi</p>
                     <p className="text-[12px] text-lp-text3 font-light">Distribusi modul baru</p>
                   </div>
                 </div>
                 <FiPlus className="text-lp-text3 group-hover:translate-x-1 transition-transform" />
               </motion.button>
               <motion.button
                 initial={{ opacity: 0, x: 20 }}
                 animate={{ opacity: 1, x: 0 }}
                 transition={{ delay: 0.1 }}
                 onClick={() => {
                   setSelectedPertemuan(null);
                   setFormData(prev => ({ ...prev, pertemuan: '' }));
                   setShowCreateTugas(true);
                 }}
                 className="flex-1 group relative overflow-hidden bg-white border border-lp-border rounded-[2rem] p-6 flex items-center justify-between hover:border-lp-text transition-all duration-500"
               >
                 <div className="flex items-center gap-4">
                   <div className="p-3 bg-lp-surface group-hover:bg-lp-text group-hover:text-white transition-all duration-500 rounded-2xl">
                     <FiFileText />
                   </div>
                   <div className="text-left">
                     <p className="font-bold text-lp-text text-[15px] tracking-tight">Buat Tugas</p>
                     <p className="text-[12px] text-lp-text3 font-light">Rancang evaluasi mahasiswa</p>
                   </div>
                 </div>
                 <FiPlus className="text-lp-text3 group-hover:translate-x-1 transition-transform" />
               </motion.button>
            </div>
          </div>

          {/* Section Divider */}
          <div className="flex items-center justify-between mb-8">
            <div className="flex items-center gap-4">
              <h2 className="text-2xl font-light text-lp-text tracking-tight">Syllabus Overview</h2>
              <div className="h-px w-32 bg-lp-border" />
            </div>
            
            <div className="flex items-center gap-3">
               <div className="bg-white border border-lp-border rounded-full p-1.5 flex items-center gap-1">
                 <button
                   onClick={() => setViewMode('grid')}
                   className={`p-2.5 rounded-full transition-all duration-300 ${viewMode === 'grid' ? 'bg-lp-text text-white shadow-lg' : 'text-lp-text3 hover:bg-lp-surface'}`}
                 >
                   <FiGrid className="text-sm" />
                 </button>
                 <button
                   onClick={() => setViewMode('list')}
                   className={`p-2.5 rounded-full transition-all duration-300 ${viewMode === 'list' ? 'bg-lp-text text-white shadow-lg' : 'text-lp-text3 hover:bg-lp-surface'}`}
                 >
                   <FiList className="text-sm" />
                 </button>
               </div>
               <button
                 onClick={fetchPertemuanList}
                 className="p-3.5 bg-white border border-lp-border rounded-full text-lp-text3 hover:text-lp-text hover:rotate-180 transition-all duration-700 shadow-sm"
               >
                 <FiClock />
               </button>
            </div>
          </div>

          {/* Pertemuan Grid */}
          <div className={`grid ${viewMode === 'grid' ? 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4' : 'grid-cols-1'} gap-6`}>
            {pertemuanList.map((pertemuan, index) => (
              <motion.div 
                key={index}
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.05 }}
                className={`group relative bg-white border ${pertemuan.has_materi && pertemuan.has_tugas ? 'border-lp-text shadow-[0_12px_32px_rgba(0,0,0,0.04)]' : 'border-lp-border'} rounded-[2.5rem] p-8 hover:shadow-[0_32px_64px_rgba(0,0,0,0.08)] transition-all duration-700 hover:-translate-y-2 cursor-pointer overflow-hidden`}
                onClick={() => fetchPertemuanDetail(pertemuan.pertemuan)}
              >
                {/* Visual Accent */}
                <div className={`absolute top-0 right-0 w-32 h-32 blur-[40px] opacity-10 transition-opacity duration-700 group-hover:opacity-30 ${pertemuan.has_materi && pertemuan.has_tugas ? 'bg-lp-text' : pertemuan.has_materi || pertemuan.has_tugas ? 'bg-lp-text2' : 'bg-transparent'}`} />
                
                <div className="relative z-10">
                  <div className="flex items-start justify-between mb-8">
                    <div>
                      <span className={`text-[10px] font-mono font-bold tracking-[0.2em] uppercase mb-1 block ${pertemuan.has_materi && pertemuan.has_tugas ? 'text-lp-text' : 'text-lp-text3'}`}>
                        Sesi {pertemuan.pertemuan}
                      </span>
                      <h3 className="text-[24px] font-normal text-lp-text tracking-tight">
                        Pertemuan {pertemuan.pertemuan}
                      </h3>
                    </div>
                    {pertemuan.has_materi && pertemuan.has_tugas ? (
                      <div className="bg-lp-text text-white text-[9px] font-mono font-bold tracking-widest px-3 py-1.5 rounded-full uppercase flex items-center gap-1">
                        <FiCheckCircle /> Lengkap
                      </div>
                    ) : (
                      <div className={`w-3 h-3 rounded-full mt-2 ${pertemuan.has_materi || pertemuan.has_tugas ? 'bg-lp-text2 ring-4 ring-lp-surface' : 'bg-lp-border'}`}></div>
                    )}
                  </div>
                  
                  <div className="space-y-3 mb-8">
                    <div className="flex items-center justify-between p-3 rounded-2xl bg-lp-surface/50 border border-lp-border/50">
                      <span className="text-[13px] text-lp-text font-light flex items-center gap-2">
                        <FiFileText className={pertemuan.has_materi ? "text-lp-text" : "text-lp-text3"} /> Modul
                      </span>
                      {pertemuan.has_materi ? (
                        <span className="text-[10px] font-bold tracking-widest uppercase px-3 py-1 text-lp-text bg-white border border-lp-border rounded-full shadow-sm">
                          Tersedia
                        </span>
                      ) : (
                        <span className="text-[10px] font-bold tracking-widest uppercase px-3 py-1 text-lp-text3">
                          Kosong
                        </span>
                      )}
                    </div>
                    <div className="flex items-center justify-between p-3 rounded-2xl bg-lp-surface/50 border border-lp-border/50">
                      <span className="text-[13px] text-lp-text font-light flex items-center gap-2">
                        <FiCalendar className={pertemuan.has_tugas ? "text-lp-text" : "text-lp-text3"} /> Tugas
                      </span>
                      {pertemuan.has_tugas ? (
                        <span className="text-[10px] font-bold tracking-widest uppercase px-3 py-1 text-lp-text bg-white border border-lp-border rounded-full shadow-sm">
                          Aktif
                        </span>
                      ) : (
                        <span className="text-[10px] font-bold tracking-widest uppercase px-3 py-1 text-lp-text3">
                          Kosong
                        </span>
                      )}
                    </div>
                  </div>
                  
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      fetchPertemuanDetail(pertemuan.pertemuan);
                    }}
                    className={`w-full py-4 rounded-2xl text-[12px] font-bold tracking-[0.1em] uppercase transition-all duration-500 flex items-center justify-center gap-2 ${!(pertemuan.has_materi || pertemuan.has_tugas) ? 'bg-white border border-lp-border text-lp-text hover:border-lp-text hover:bg-lp-surface' : 'bg-lp-text text-white hover:bg-lp-atext shadow-md hover:shadow-xl hover:-translate-y-0.5'}`}
                  >
                    {!(pertemuan.has_materi || pertemuan.has_tugas) ? (
                      <>Atur Sesi <FiPlus /></>
                    ) : (
                      <>Kelola Sesi <FiChevronRight /></>
                    )}
                  </button>
                </div>
              </motion.div>
            ))}
          </div>

          {/* Modals Container */}
          <AnimatePresence>
            {/* Modal Upload Materi */}
            {showUploadMateri && (
              <div className="fixed inset-0 bg-lp-text/20 backdrop-blur-md flex items-center justify-center p-4 lg:pl-[260px] z-[1000] overflow-y-auto transition-all duration-500">
                <motion.div 
                  initial={{ opacity: 0, scale: 0.9, y: 20 }}
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.9, y: 20 }}
                  className="bg-white border border-lp-border rounded-[2.5rem] p-6 md:p-10 w-full max-w-2xl my-auto shadow-[0_32px_64px_rgba(0,0,0,0.12)] relative max-h-[90dvh] overflow-y-auto custom-scrollbar"
                >
                  <div className="absolute top-8 right-8">
                     <button onClick={() => setShowUploadMateri(false)} className="w-10 h-10 rounded-full bg-lp-surface border border-lp-border flex items-center justify-center text-lp-text hover:bg-lp-text hover:text-white transition-all duration-300">✕</button>
                  </div>

                  <div className="mb-10">
                    <span className="text-[11px] font-mono font-medium tracking-[0.2em] uppercase text-lp-text3 mb-3 block">LECTURE MATERIAL</span>
                    <h3 className="text-3xl md:text-4xl font-normal text-lp-text tracking-tight leading-none italic">
                      Upload Materi Baru
                    </h3>
                  </div>
                  
                  <form onSubmit={handleUploadMateri} className="space-y-8">
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
                      <div>
                        <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">Pertemuan *</label>
                        <select
                          value={formData.pertemuan}
                          onChange={(e) => setFormData(prev => ({ ...prev, pertemuan: e.target.value }))}
                          className={`w-full font-light text-[15px] border border-lp-border rounded-2xl px-5 py-4 focus:outline-none focus:ring-2 focus:ring-lp-accent/5 focus:border-lp-text transition-all duration-500 appearance-none ${selectedPertemuan ? 'bg-lp-surface/50 text-lp-text2 cursor-not-allowed' : 'bg-lp-surface text-lp-text'}`}
                          required
                          disabled={!!selectedPertemuan}
                        >
                          <option value="">Pilih Pertemuan</option>
                          {Array.from({ length: 16 }, (_, i) => i + 1).map(pertemuan => (
                            <option key={pertemuan} value={pertemuan}>Pertemuan {pertemuan}</option>
                          ))}
                        </select>
                      </div>
                      
                      <div>
                        <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">Judul Materi *</label>
                        <input
                          type="text"
                          value={formData.title}
                          onChange={(e) => setFormData(prev => ({ ...prev, title: e.target.value }))}
                          className="w-full bg-lp-surface font-light text-[15px] border border-lp-border rounded-2xl px-5 py-4 focus:outline-none focus:ring-2 focus:ring-lp-accent/5 focus:border-lp-text transition-all duration-500"
                          placeholder="Introduction to Calculus..."
                          required
                        />
                      </div>
                    </div>
                    
                    <div>
                      <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">Deskripsi</label>
                      <textarea
                        value={formData.desc}
                        onChange={(e) => setFormData(prev => ({ ...prev, desc: e.target.value }))}
                        className="w-full bg-lp-surface font-light text-[15px] border border-lp-border rounded-2xl px-5 py-4 focus:outline-none focus:ring-2 focus:ring-lp-accent/5 focus:border-lp-text transition-all duration-500 resize-none whitespace-pre-wrap"
                        rows="3"
                        placeholder="Detail tentang materi yang dibagikan..."
                      />
                    </div>
                    
                    <div>
                      <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">File Materi (Opsional)</label>
                      <div className="relative group">
                        <input
                          type="file"
                          onChange={(e) => handleFileChange(e, 'file')}
                          className="hidden"
                          id="file-upload-materi"
                          accept=".pdf,.ppt,.pptx,.doc,.docx,.zip,.jpg,.jpeg,.png"
                        />
                        <label 
                          htmlFor="file-upload-materi"
                          className={`
                            relative flex flex-col items-center justify-center w-full min-h-[180px] 
                            p-10 border-2 border-dashed rounded-[2rem] cursor-pointer
                            transition-all duration-700 ease-[cubic-bezier(0.16,1,0.3,1)]
                            ${formData.file 
                              ? 'border-lp-text bg-lp-surface' 
                              : 'border-lp-border bg-lp-surface hover:border-lp-text hover:bg-lp-bg'
                            }
                          `}
                        >
                           <div className="flex flex-col items-center text-center">
                              <div className={`
                                w-14 h-14 rounded-2xl flex items-center justify-center mb-5 group-hover:scale-110 transition-transform duration-500
                                ${formData.file ? 'bg-lp-text text-white' : 'bg-white border border-lp-border text-lp-text3'}
                              `}>
                                {formData.file ? <FiCheckCircle className="text-2xl" /> : <FiUpload className="text-2xl animate-bounce" />}
                              </div>
                              {formData.file ? (
                                <>
                                  <p className="text-[16px] font-normal text-lp-text mb-1 tracking-tight italic">File Siap Diunggah</p>
                                  <p className="text-[12px] text-lp-text3 font-mono uppercase">{formData.file.name}</p>
                                </>
                              ) : (
                                <>
                                  <p className="text-[16px] font-normal text-lp-text mb-1 tracking-tight">Tarik file ke sini atau <em className="italic underline">telusuri</em>.</p>
                                  <p className="text-[10px] text-lp-text3 font-mono font-medium tracking-widest mt-2 uppercase">PDF · PPT · DOC · ZIP</p>
                                </>
                              )}
                           </div>
                        </label>
                      </div>
                    </div>
                    
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-4">
                      <button
                        type="button"
                        onClick={() => setShowUploadMateri(false)}
                        className="px-8 py-4 border border-lp-border text-lp-text2 rounded-full text-[13px] font-bold hover:bg-lp-surface transition-all duration-300 uppercase tracking-widest"
                      >
                        Batal
                      </button>
                      <button
                        type="submit"
                        disabled={submitting}
                        className="bg-lp-text text-lp-bg px-8 py-4 rounded-full text-[13px] font-bold hover:bg-lp-atext hover:-translate-y-1 disabled:opacity-40 transition-all duration-500 uppercase tracking-widest flex items-center justify-center gap-3 shadow-[0_12px_24px_rgba(0,0,0,0.1)]"
                      >
                        {submitting && <div className="w-4 h-4 border-2 border-lp-bg/30 border-t-lp-bg rounded-full animate-spin"></div>}
                        <span>{submitting ? 'MEMPUBLIKASIKAN' : 'PUBLIKASI MATERI'}</span>
                      </button>
                    </div>
                  </form>
                </motion.div>
              </div>
            )}

            {/* Modal Buat Tugas */}
            {showCreateTugas && (
              <div className="fixed inset-0 bg-lp-text/20 backdrop-blur-md flex items-center justify-center p-4 lg:pl-[260px] z-[1000] overflow-y-auto transition-all duration-500">
                <motion.div 
                  initial={{ opacity: 0, scale: 0.9, y: 20 }}
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.9, y: 20 }}
                  className="bg-white border border-lp-border rounded-[2.5rem] p-6 md:p-10 w-full max-w-2xl my-auto shadow-[0_32px_64px_rgba(0,0,0,0.12)] relative max-h-[90dvh] overflow-y-auto custom-scrollbar"
                >
                  <div className="absolute top-8 right-8">
                     <button onClick={() => setShowCreateTugas(false)} className="w-10 h-10 rounded-full bg-lp-surface border border-lp-border flex items-center justify-center text-lp-text hover:bg-lp-text hover:text-white transition-all duration-300">✕</button>
                  </div>

                  <div className="mb-10">
                    <span className="text-[11px] font-mono font-medium tracking-[0.2em] uppercase text-lp-text3 mb-3 block">TUGAS PENILAIAN</span>
                    <h3 className="text-3xl md:text-4xl font-normal text-lp-text tracking-tight leading-none italic">
                      Buat Tugas Baru
                    </h3>
                  </div>
                  
                  <form onSubmit={handleCreateTugas} className="space-y-8">
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
                      <div>
                        <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">Pertemuan *</label>
                        <select
                          value={formData.pertemuan}
                          onChange={(e) => setFormData(prev => ({ ...prev, pertemuan: e.target.value }))}
                          className={`w-full font-light text-[15px] border border-lp-border rounded-2xl px-5 py-4 focus:outline-none focus:ring-2 focus:ring-lp-accent/5 focus:border-lp-text transition-all duration-500 appearance-none ${selectedPertemuan ? 'bg-lp-surface/50 text-lp-text2 cursor-not-allowed' : 'bg-lp-surface text-lp-text'}`}
                          required
                          disabled={!!selectedPertemuan}
                        >
                          <option value="">Pilih Pertemuan</option>
                          {Array.from({ length: 16 }, (_, i) => i + 1).map(pertemuan => (
                            <option key={pertemuan} value={pertemuan}>Pertemuan {pertemuan}</option>
                          ))}
                        </select>
                      </div>
                      
                      <div>
                        <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">Judul Tugas *</label>
                        <input
                          type="text"
                          value={formData.title}
                          onChange={(e) => setFormData(prev => ({ ...prev, title: e.target.value }))}
                          className="w-full bg-lp-surface font-light text-[15px] border border-lp-border rounded-2xl px-5 py-4 focus:outline-none focus:ring-2 focus:ring-lp-accent/5 focus:border-lp-text transition-all duration-500"
                          placeholder="Project Milestone 1..."
                          required
                        />
                      </div>
                    </div>
                    
                    <div>
                      <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">Instruksi Tugas *</label>
                      <textarea
                        value={formData.desc}
                        onChange={(e) => setFormData(prev => ({ ...prev, desc: e.target.value }))}
                        className="w-full bg-lp-surface font-light text-[15px] border border-lp-border rounded-2xl px-5 py-4 focus:outline-none focus:ring-2 focus:ring-lp-accent/5 focus:border-lp-text transition-all duration-500 resize-none whitespace-pre-wrap"
                        rows="4"
                        placeholder="Deskripsikan tantangan bagi mahasiswa..."
                        required
                      />
                    </div>
                    
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
                      <div>
                        <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">Tenggat Waktu Pengumpulan</label>
                        <input
                          type="datetime-local"
                          value={formData.due_date}
                          onChange={(e) => setFormData(prev => ({ ...prev, due_date: e.target.value }))}
                          className="w-full bg-lp-surface font-light text-[15px] border border-lp-border rounded-2xl px-5 py-4 focus:outline-none focus:ring-2 focus:ring-lp-accent/5 focus:border-lp-text transition-all duration-500 appearance-none"
                        />
                      </div>
                      
                      <div>
                        <label className="block text-[11px] font-mono font-semibold text-lp-text3 mb-3 tracking-widest uppercase">File Referensi <span className="font-sans lowercase opacity-50 font-normal ml-1">(Opsional)</span></label>
                        <div className="relative group">
                          <input
                            type="file"
                            onChange={(e) => handleFileChange(e, 'file_tugas')}
                            className="hidden"
                            id="file-upload-tugas"
                            accept=".pdf,.doc,.docx,.zip,.jpg,.jpeg,.png"
                          />
                          <label 
                            htmlFor="file-upload-tugas"
                            className={`
                              relative flex flex-col items-center justify-center w-full min-h-[140px] 
                              p-6 border-2 border-dashed rounded-[2rem] cursor-pointer
                              transition-all duration-700 ease-[cubic-bezier(0.16,1,0.3,1)]
                              ${formData.file_tugas 
                                ? 'border-lp-text bg-lp-surface' 
                                : 'border-lp-border bg-lp-surface hover:border-lp-text hover:bg-lp-bg'
                              }
                            `}
                          >
                             <div className="flex flex-col items-center text-center">
                                <div className={`
                                  w-10 h-10 rounded-xl flex items-center justify-center mb-3 group-hover:scale-110 transition-transform duration-500
                                  ${formData.file_tugas ? 'bg-lp-text text-white' : 'bg-white border border-lp-border text-lp-text3'}
                                `}>
                                  {formData.file_tugas ? <FiCheckCircle /> : <FiUpload />}
                                </div>
                                {formData.file_tugas ? (
                                  <p className="text-[12px] text-lp-text font-mono uppercase truncate max-w-[160px]">{formData.file_tugas.name}</p>
                                ) : (
                                  <p className="text-[12px] text-lp-text3 font-medium uppercase tracking-[0.1em]">Upload Panduan</p>
                                )}
                             </div>
                          </label>
                        </div>
                      </div>
                    </div>
                    
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-4">
                      <button
                        type="button"
                        onClick={() => setShowCreateTugas(false)}
                        className="px-8 py-4 border border-lp-border text-lp-text2 rounded-full text-[13px] font-bold hover:bg-lp-surface transition-all duration-300 uppercase tracking-widest"
                      >
                        Batal
                      </button>
                      <button
                        type="submit"
                        disabled={submitting}
                        className="bg-lp-text text-lp-bg px-8 py-4 rounded-full text-[13px] font-bold hover:bg-lp-atext hover:-translate-y-1 disabled:opacity-40 transition-all duration-500 uppercase tracking-widest flex items-center justify-center gap-3 shadow-[0_12px_24px_rgba(0,0,0,0.1)]"
                      >
                        {submitting && <div className="w-4 h-4 border-2 border-lp-bg/30 border-t-lp-bg rounded-full animate-spin"></div>}
                        <span>{submitting ? 'MENGAKTIFKAN' : 'AKTIFKAN TUGAS'}</span>
                      </button>
                    </div>
                  </form>
                </motion.div>
              </div>
            )}

            {/* Modal Detail Pertemuan */}
            {showDetailPertemuan && (
              <div className="fixed inset-0 bg-lp-text/30 backdrop-blur-md flex items-center justify-center p-4 lg:pl-[260px] z-[1000] overflow-y-auto transition-all duration-500">
                <motion.div 
                  initial={{ opacity: 0, y: 50 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: 50 }}
                  className="bg-lp-bg border border-lp-border rounded-[3rem] p-6 md:p-10 w-full max-w-5xl my-auto shadow-[0_64px_128px_rgba(0,0,0,0.15)] relative overflow-hidden max-h-[95dvh] overflow-y-auto custom-scrollbar"
                >
                  <div className="absolute top-0 right-0 w-64 h-64 bg-lp-text/5 blur-[80px] rounded-full translate-x-1/2 -translate-y-1/2" />
                  
                  <div className="flex items-start justify-between mb-12 relative z-10">
                    <div>
                      <span className="text-[11px] font-mono font-bold tracking-[0.2em] text-lp-text3 uppercase mb-4 block">DETAIL SESI</span>
                      <h3 className="text-4xl md:text-5xl font-light text-lp-text tracking-tight mb-3">
                        Pertemuan {selectedPertemuan}
                      </h3>
                      <div className="flex items-center gap-2 text-lp-text2 font-light">
                        <FiBookOpen className="text-lp-text3" />
                        <span>{courseName || courseId}</span>
                      </div>
                    </div>
                    <button
                      onClick={() => setShowDetailPertemuan(false)}
                      className="w-12 h-12 rounded-full bg-white border border-lp-border flex items-center justify-center text-lp-text hover:bg-lp-text hover:text-white transition-all duration-500 shadow-sm"
                    >
                      <FiXCircle className="text-xl" />
                    </button>
                  </div>
                  
                  {detailLoading ? (
                    <div className="flex flex-col items-center justify-center py-20 relative z-10">
                      <div className="w-12 h-12 border-2 border-lp-text/10 border-t-lp-text rounded-full animate-spin mb-6"></div>
                      <p className="text-[11px] font-mono font-bold tracking-widest text-lp-text3 uppercase">Sinkronisasi Modul...</p>
                    </div>
                  ) : (
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-12 relative z-10">
                      
                      {/* Materi Section */}
                      <div className="space-y-8">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3">
                            <h4 className="text-[11px] font-mono font-bold tracking-[0.2em] uppercase text-lp-text3">Modul</h4>
                            <div className="h-px w-12 bg-lp-border" />
                          </div>
                          <button
                            onClick={() => {
                              setFormData(prev => ({ ...prev, pertemuan: selectedPertemuan }));
                              setShowUploadMateri(true);
                              setShowDetailPertemuan(false);
                            }}
                            className="text-[10px] font-mono font-bold tracking-widest uppercase text-lp-text2 hover:text-lp-accent flex items-center gap-1.5"
                          >
                            <FiPlus /> Upload Materi
                          </button>
                        </div>
                        
                        <div className="space-y-4 max-h-[400px] overflow-y-auto pr-2 custom-scrollbar">
                          {pertemuanDetail.materi && pertemuanDetail.materi.length > 0 ? (
                            pertemuanDetail.materi.map((materi, index) => (
                              <div key={index} className="group bg-white border border-lp-border rounded-[2rem] p-6 hover:shadow-xl hover:-translate-y-1 transition-all duration-500">
                                <div className="flex justify-between items-start gap-4">
                                  <div className="flex-1 min-w-0">
                                    <p className="text-[10px] font-mono font-bold text-lp-text3 tracking-widest uppercase mb-2">MOD-{materi.id}</p>
                                    <h5 className="text-[18px] font-normal text-lp-text tracking-tight mb-2 truncate italic">{materi.title}</h5>
                                    <p className="text-[13px] text-lp-text2 font-light line-clamp-2 whitespace-pre-wrap">{materi.desc || "Tidak ada deskripsi."}</p>
                                  </div>
                                  <div className="flex flex-col gap-2">
                                    {materi.file_path && (
                                      <a 
                                        href={resolveBackendAssetUrl(materi.file_path)}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        className="p-3 bg-lp-surface text-lp-text3 rounded-2xl hover:bg-lp-text hover:text-white transition-all shadow-sm"
                                      >
                                        <FiDownload />
                                      </a>
                                    )}
                                    <button
                                      onClick={() => handleDeleteMateri(materi.id)}
                                      className="p-3 bg-lp-surface text-lp-text2/60 rounded-2xl hover:bg-lp-text hover:text-white transition-all shadow-sm"
                                    >
                                      <FiTrash2 />
                                    </button>
                                  </div>
                                </div>
                              </div>
                            ))
                          ) : (
                            <div className="py-16 text-center border-2 border-dashed border-lp-border rounded-[2rem]">
                              <FiFileText className="text-4xl text-lp-text/10 mx-auto mb-4" />
                              <p className="text-[11px] font-mono font-bold text-lp-text3 uppercase">Belum ada materi</p>
                            </div>
                          )}
                        </div>
                      </div>

                      {/* Tugas Section */}
                      <div className="space-y-8">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3">
                            <h4 className="text-[11px] font-mono font-bold tracking-[0.2em] uppercase text-lp-text3">Evaluasi</h4>
                            <div className="h-px w-12 bg-lp-border" />
                          </div>
                          <button
                            onClick={() => {
                              setFormData(prev => ({ ...prev, pertemuan: selectedPertemuan }));
                              setShowCreateTugas(true);
                              setShowDetailPertemuan(false);
                            }}
                            className="text-[10px] font-mono font-bold tracking-widest uppercase text-lp-text2 hover:text-lp-accent flex items-center gap-1.5"
                          >
                            <FiPlus /> Upload Tugas
                          </button>
                        </div>
                        
                        <div className="space-y-4 max-h-[400px] overflow-y-auto pr-2 custom-scrollbar">
                          {pertemuanDetail.tugas && pertemuanDetail.tugas.length > 0 ? (
                            pertemuanDetail.tugas.map((tugas, index) => (
                              <div key={index} className="group bg-white border border-lp-border rounded-[2rem] p-6 hover:shadow-xl hover:-translate-y-1 transition-all duration-500">
                                <div className="flex justify-between items-start gap-4 mb-4">
                                  <div className="flex-1 min-w-0">
                                    <p className="text-[10px] font-mono font-bold text-lp-text3 tracking-widest uppercase mb-2">TUGAS-{tugas.id}</p>
                                    <h5 className="text-[18px] font-normal text-lp-text tracking-tight italic">{tugas.title}</h5>
                                  </div>
                                  <div className="flex flex-col gap-2">
                                    <Link 
                                      to={`/dosen/penilaian/${courseId}?taskId=${tugas.id}`}
                                      className="p-3 bg-lp-surface text-lp-text2 rounded-2xl hover:bg-lp-text hover:text-white transition-all shadow-sm flex items-center justify-center"
                                    >
                                      <FiEye />
                                    </Link>
                                    <button
                                      onClick={() => handleDeleteTugas(tugas.id)}
                                      className="p-3 bg-lp-surface text-lp-text2/60 rounded-2xl hover:bg-lp-text hover:text-white transition-all shadow-sm"
                                    >
                                      <FiTrash2 />
                                    </button>
                                  </div>
                                </div>
                                <div className="space-y-2">
                                  <div className="flex items-center gap-2 text-[12px] text-lp-text2 font-light">
                                    <FiClock className="opacity-60" />
                                    <span>Deadline: {tugas.due_date ? new Date(tugas.due_date).toLocaleString('id-ID') : 'Fleksibel'}</span>
                                  </div>
                                  {tugas.file_path && (
                                    <div className="flex items-center gap-2 text-[12px] text-lp-text2 font-light">
                                      <FiFile className="opacity-60" />
                                      <a href={resolveBackendAssetUrl(tugas.file_path)} className="underline hover:text-lp-text2">File Referensi</a>
                                    </div>
                                  )}
                                </div>
                              </div>
                            ))
                          ) : (
                            <div className="py-16 text-center border-2 border-dashed border-lp-border rounded-[2rem]">
                              <FiCalendar className="text-4xl text-lp-text/10 mx-auto mb-4" />
                              <p className="text-[11px] font-mono font-bold text-lp-text3 uppercase">Belum ada tugas</p>
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  )}
                </motion.div>
              </div>
            )}
          </AnimatePresence>
        </div>
      </div>
    </div>
  )
}

export default KelolaMatkulDosen
