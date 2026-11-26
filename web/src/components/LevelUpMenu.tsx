import { useEffect } from 'react'
import type { LevelUpInfo } from '../net/useGameSocket'

interface Props {
  offer: LevelUpInfo
  onChoose(index: number): void
}

const KEYS = ['Digit1', 'Digit2', 'Digit3', 'Numpad1', 'Numpad2', 'Numpad3']

/** 升級三選一：疊在遊戲畫面上，滑鼠點選或按 1 / 2 / 3。 */
export function LevelUpMenu({ offer, onChoose }: Props) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const i = KEYS.indexOf(e.code) % 3
      if (i < 0 || KEYS.indexOf(e.code) < 0 || i >= offer.options.length) return
      e.preventDefault()
      onChoose(i)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [offer, onChoose])

  return (
    <div className="level-up" role="dialog" aria-labelledby="level-up-title">
      <h2 id="level-up-title">升級！Lv {offer.level}</h2>
      <p className="muted">選一個強化（滑鼠點選或按 1 / 2 / 3）</p>
      <ol className="level-up-options">
        {offer.options.map((o, i) => (
          <li key={o.id}>
            <button type="button" onClick={() => onChoose(i)} data-testid={`upgrade-${i}`}>
              <kbd>{i + 1}</kbd>
              <strong>{o.title}</strong>
              <span className="rank">{o.rank > 1 ? `Lv ${o.rank}` : '新！'}</span>
              <span className="desc">{o.description}</span>
            </button>
          </li>
        ))}
      </ol>
    </div>
  )
}
