import { X } from 'lucide-react'
import type { ReactNode } from 'react'

type SlideOverProps = {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
}

export default function SlideOver({ open, onClose, title, children }: SlideOverProps) {
  return (
    <>
      <button
        aria-label="关闭"
        className={`fixed inset-0 z-40 bg-slate-950/40 transition-opacity duration-300 ${
          open ? 'opacity-100' : 'pointer-events-none opacity-0'
        }`}
        type="button"
        onClick={onClose}
      />
      <aside
        aria-hidden={!open}
        aria-label={title}
        aria-modal="true"
        className={`glass-panel fixed inset-y-0 right-0 z-50 flex w-full flex-col overflow-hidden rounded-none border-y-0 border-r-0 shadow-2xl shadow-slate-950/20 transition-transform duration-300 sm:w-1/2 ${
          open ? 'translate-x-0' : 'translate-x-full'
        }`}
        role="dialog"
      >
        <div className="flex items-center justify-between border-b border-white/50 px-5 py-4">
          <h2 className="min-w-0 break-words text-base font-semibold text-slate-950">{title}</h2>
          <button aria-label="关闭" className="icon-btn shrink-0" type="button" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">{children}</div>
      </aside>
    </>
  )
}
