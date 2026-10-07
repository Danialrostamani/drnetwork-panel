import Clipboard from 'clipboard'

// copyText puts text on the clipboard. The asynchronous clipboard API needs a
// secure context, which a panel reached over plain HTTP is not; the fallback
// copies through a hidden button inside the open dialog, as a dialog keeps
// the focus to itself.
export async function copyText(text: string, container?: HTMLElement | null): Promise<boolean> {
  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Denied: try the other way.
    }
  }
  return new Promise<boolean>(resolve => {
    const host = container ?? document.body
    const button = document.createElement('button')
    button.type = 'button'
    button.style.display = 'none'
    host.appendChild(button)
    const clipboard = new Clipboard(button, { text: () => text, container: container ?? undefined })
    const done = (ok: boolean) => {
      clipboard.destroy()
      button.remove()
      resolve(ok)
    }
    clipboard.on('success', () => done(true))
    clipboard.on('error', () => done(false))
    button.click()
  })
}
