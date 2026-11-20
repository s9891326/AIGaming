import { useEffect } from 'react'

type Dir = 'up' | 'down' | 'left' | 'right'

/** 使用 KeyboardEvent.code（實體按鍵位置），所以注音 / 其他鍵盤配置下 WASD 位置不變。 */
const KEY_TO_DIR: Record<string, Dir> = {
  KeyW: 'up',
  ArrowUp: 'up',
  KeyS: 'down',
  ArrowDown: 'down',
  KeyA: 'left',
  ArrowLeft: 'left',
  KeyD: 'right',
  ArrowRight: 'right',
}

/**
 * 依按住的方向鍵算出移動向量（畫面座標：x 往右、y 往下）。
 * 斜向會正規化成長度 1，所以斜走不會比直走快；相反方向同時按住會互相抵銷。
 */
export function moveVector(pressed: ReadonlySet<Dir>): { x: number; y: number } {
  const x = (pressed.has('right') ? 1 : 0) - (pressed.has('left') ? 1 : 0)
  const y = (pressed.has('down') ? 1 : 0) - (pressed.has('up') ? 1 : 0)
  if (x !== 0 && y !== 0) return { x: x * Math.SQRT1_2, y: y * Math.SQRT1_2 }
  return { x, y }
}

/**
 * 監聽 WASD / 方向鍵，方向改變時呼叫 onChange（呼叫端負責「沒變就不送」）。
 * 視窗失去焦點時視為放開所有按鍵，避免切換視窗後角色一直走。
 */
export function useKeyboardMove(onChange: (x: number, y: number) => void, enabled = true) {
  useEffect(() => {
    if (!enabled) return
    const pressed = new Set<Dir>()
    const emit = () => {
      const v = moveVector(pressed)
      onChange(v.x, v.y)
    }

    const isTyping = (e: KeyboardEvent) =>
      e.target instanceof HTMLElement &&
      (e.target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName))

    const onKeyDown = (e: KeyboardEvent) => {
      const dir = KEY_TO_DIR[e.code]
      if (!dir || isTyping(e) || e.ctrlKey || e.metaKey || e.altKey) return
      e.preventDefault() // 方向鍵不要捲動頁面
      if (pressed.has(dir)) return // 按住時的自動重複
      pressed.add(dir)
      emit()
    }
    const onKeyUp = (e: KeyboardEvent) => {
      const dir = KEY_TO_DIR[e.code]
      if (!dir || !pressed.delete(dir)) return
      emit()
    }
    const onBlur = () => {
      if (pressed.size === 0) return
      pressed.clear()
      emit()
    }

    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('keyup', onKeyUp)
    window.addEventListener('blur', onBlur)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('keyup', onKeyUp)
      window.removeEventListener('blur', onBlur)
      onChange(0, 0)
    }
  }, [onChange, enabled])
}
