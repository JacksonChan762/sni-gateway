import { useState, useCallback, useEffect, useId, useRef } from 'react'
import { AlertTriangle, X } from 'lucide-react'

interface ConfirmOptions {
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  dangerous?: boolean
}

interface PendingConfirm extends ConfirmOptions {
  resolve: (value: boolean) => void
}

interface ConfirmModalProps extends ConfirmOptions {
  onConfirm: () => void
  onCancel: () => void
}

function ConfirmModal({
  title,
  message,
  confirmText = 'Confirm',
  cancelText = 'Cancel',
  dangerous = false,
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  const titleId = useId()
  const messageId = useId()
  const dialog = useRef<HTMLDivElement>(null)
  const cancelButton = useRef<HTMLButtonElement>(null)
  const cancel = useRef(onCancel)
  cancel.current = onCancel

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null
    cancelButton.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') cancel.current()
      if (event.key === 'Tab') {
        const buttons = dialog.current?.querySelectorAll<HTMLButtonElement>('button')
        if (!buttons?.length) return
        const first = buttons[0]
        const last = buttons[buttons.length - 1]
        if (event.shiftKey && document.activeElement === first) {
          last.focus()
          event.preventDefault()
        } else if (!event.shiftKey && document.activeElement === last) {
          first.focus()
          event.preventDefault()
        }
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      previousFocus?.focus()
    }
  }, [])

  return (
    <div className="fixed inset-0 z-[150] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div ref={dialog} role="dialog" aria-modal="true" aria-labelledby={titleId} aria-describedby={messageId} className="bg-gray-900 border border-gray-700 rounded-2xl w-full max-w-sm shadow-2xl">
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-800">
          <div className="flex items-center gap-2">
            {dangerous && <AlertTriangle size={16} className="text-red-400" />}
            <h3 id={titleId} className="font-semibold text-white">{title}</h3>
          </div>
          <button
            className="text-gray-500 hover:text-gray-300 transition-colors p-1"
            onClick={onCancel}
            aria-label="Close confirmation"
          >
            <X size={16} />
          </button>
        </div>
        <div className="px-5 py-4">
          <p id={messageId} className="text-gray-300 text-sm leading-relaxed">{message}</p>
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-gray-800">
          <button ref={cancelButton} className="btn-secondary" onClick={onCancel}>
            {cancelText}
          </button>
          <button
            className={dangerous ? 'btn-danger' : 'btn-primary'}
            onClick={onConfirm}
          >
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  )
}

export function useConfirm() {
  const [pending, setPending] = useState<PendingConfirm | null>(null)

  const confirm = useCallback((opts: ConfirmOptions): Promise<boolean> => {
    return new Promise(resolve => {
      setPending({ ...opts, resolve })
    })
  }, [])

  const modal = pending ? (
    <ConfirmModal
      {...pending}
      onConfirm={() => {
        pending.resolve(true)
        setPending(null)
      }}
      onCancel={() => {
        pending.resolve(false)
        setPending(null)
      }}
    />
  ) : null

  return { confirm, modal }
}
