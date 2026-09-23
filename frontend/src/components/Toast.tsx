import { useState, useCallback, createContext, useContext, useRef } from 'react'
import { CheckCircle, XCircle, AlertCircle, X } from 'lucide-react'

type ToastType = 'success' | 'error' | 'info'

interface Toast {
  id: number
  type: ToastType
  text: string
}

interface ToastContextValue {
  success: (text: string) => void
  error: (text: string) => void
  info: (text: string) => void
}

const ToastContext = createContext<ToastContextValue>({
  success: () => {},
  error: () => {},
  info: () => {},
})

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const idRef = useRef(0)

  const dismiss = useCallback((id: number) => {
    setToasts(prev => prev.filter(t => t.id !== id))
  }, [])

  const add = useCallback(
    (type: ToastType, text: string) => {
      const id = ++idRef.current
      setToasts(prev => [...prev, { id, type, text }])
      setTimeout(() => dismiss(id), 4500)
    },
    [dismiss]
  )

  return (
    <ToastContext.Provider
      value={{
        success: t => add('success', t),
        error: t => add('error', t),
        info: t => add('info', t),
      }}
    >
      {children}
      <div className="fixed top-4 right-4 z-[200] flex flex-col gap-2 w-80">
        {toasts.map(t => (
          <div
            key={t.id}
            role={t.type === 'error' ? 'alert' : 'status'}
            className={`flex items-start gap-3 px-4 py-3 rounded-xl shadow-2xl text-sm border animate-slide-in ${
              t.type === 'success'
                ? 'bg-green-950 border-green-700/60 text-green-100'
                : t.type === 'error'
                ? 'bg-red-950 border-red-700/60 text-red-100'
                : 'bg-gray-900 border-gray-700 text-gray-100'
            }`}
          >
            {t.type === 'success' && (
              <CheckCircle size={16} className="text-green-400 mt-0.5 flex-shrink-0" />
            )}
            {t.type === 'error' && (
              <XCircle size={16} className="text-red-400 mt-0.5 flex-shrink-0" />
            )}
            {t.type === 'info' && (
              <AlertCircle size={16} className="text-blue-400 mt-0.5 flex-shrink-0" />
            )}
            <span className="flex-1 leading-relaxed">{t.text}</span>
            <button
              onClick={() => dismiss(t.id)}
              aria-label="Dismiss notification"
              className="opacity-50 hover:opacity-100 transition-opacity mt-0.5 flex-shrink-0"
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export const useToast = () => useContext(ToastContext)
