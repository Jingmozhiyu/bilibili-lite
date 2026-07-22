import { createContext, useContext } from 'react'

export const UploadContext = createContext<(() => void) | null>(null)

export function useUploadPanel() {
  const openUpload = useContext(UploadContext)
  if (!openUpload) throw new Error('useUploadPanel must be used inside the application layout')
  return openUpload
}
