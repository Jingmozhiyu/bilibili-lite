import { useState } from 'react'
import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { useAuth } from './auth/useAuth'
import { AppHeader } from './components/AppHeader'
import { UploadPanel } from './components/UploadPanel'
import { UploadContext } from './context/UploadContext'
import { HistoryPage } from './pages/HistoryPage'
import { HomePage } from './pages/HomePage'
import { AdminReviewPage } from './pages/AdminReviewPage'
import { SearchPage } from './pages/SearchPage'
import { VideoPage } from './pages/VideoPage'
import { UserPage } from './pages/UserPage'
import './styles/shell.css'
import './styles/home.css'
import './styles/video.css'
import './styles/history.css'
import './styles/space.css'
import './styles/search.css'
import './styles/admin.css'

function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<HomePage />} />
        <Route path="video/:bvid" element={<VideoPage />} />
        <Route path="space/:userId" element={<UserPage />} />
        <Route path="history/:kind" element={<HistoryPage />} />
        <Route path="search" element={<SearchPage />} />
        <Route path="admin" element={<Navigate to="/admin/reviews" replace />} />
        <Route path="admin/reviews" element={<AdminReviewPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

function AppLayout() {
  const { session } = useAuth()
  const [uploadOpen, setUploadOpen] = useState(false)
  const [authOpen, setAuthOpen] = useState(false)
  const requestUpload = () => {
    if (session) setUploadOpen(true)
    else setAuthOpen(true)
  }
  return (
    <UploadContext.Provider value={requestUpload}>
      <div className="app-shell">
        <AppHeader authOpen={authOpen} onAuthOpenChange={setAuthOpen} onUpload={requestUpload} />
        <Outlet />
        <UploadPanel open={uploadOpen} onClose={() => setUploadOpen(false)} />
      </div>
    </UploadContext.Provider>
  )
}

export default App
