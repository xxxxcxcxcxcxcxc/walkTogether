import { useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, LoaderCircle, Pencil, Plus, Trash2 } from 'lucide-react'
import { useTelegramTheme } from '../../hooks/useTelegramTheme'
import {
  addAvailabilityInterval, addAvailabilityRule, deleteAvailabilityInterval, deleteAvailabilityRule, updateAvailabilityInterval,
  getAvailability, getAvailabilityPrivacySettings, getAvailabilityRules, getFriendAvailability, getFriends,
  setAvailabilityDay, updateAvailabilityPrivacySettings,
  type AvailabilityDay, type AvailabilityRule, type FriendAvailability, type FriendProfile,
} from '../../services/auth'

const weekdayNames = ['Воскресенье', 'Понедельник', 'Вторник', 'Среда', 'Четверг', 'Пятница', 'Суббота']
const monthFormatter = new Intl.DateTimeFormat('ru-RU', { month: 'long', year: 'numeric' })
const dayFormatter = new Intl.DateTimeFormat('ru-RU', { weekday: 'long', day: 'numeric', month: 'long' })
const dateKey = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
const dateFromKey = (key: string) => {
  const [year, month, day] = key.split('-').map(Number)
  return new Date(year, month - 1, day)
}

export function AvailabilityPage() {
  const { bgColor, textColor, hintColor, secondaryBgColor } = useTelegramTheme()
  const [month, setMonth] = useState(() => new Date(new Date().getFullYear(), new Date().getMonth(), 1))
  const [selectedDate, setSelectedDate] = useState(() => dateKey(new Date()))
  const [days, setDays] = useState<AvailabilityDay[]>([])
  const [rules, setRules] = useState<AvailabilityRule[]>([])
  const [friends, setFriends] = useState<FriendProfile[]>([])
  const [friendDays, setFriendDays] = useState<FriendAvailability[]>([])
  const [friendIDs, setFriendIDs] = useState<string[]>([])
  const [timezone, setTimezone] = useState('Europe/Moscow')
  const [visibility, setVisibility] = useState<'friends_exact' | 'selected_exact' | 'busy_only'>('friends_exact')
  const [exactFor, setExactFor] = useState<string[]>([])
  const [startTime, setStartTime] = useState('16:00')
  const [endTime, setEndTime] = useState('20:00')
  const [intervalKind, setIntervalKind] = useState<'free'|'busy'>('free')
  const [editingIntervalID, setEditingIntervalID] = useState('')
  const [ruleDay, setRuleDay] = useState(1)
  const [ruleEndsOn, setRuleEndsOn] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const from = dateKey(month)
  const to = dateKey(new Date(month.getFullYear(), month.getMonth() + 1, 0))
  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [calendar, currentRules, currentFriends, privacy] = await Promise.all([
        getAvailability(from, to), getAvailabilityRules(), getFriends(), getAvailabilityPrivacySettings(),
      ])
      setDays(calendar.days)
      setTimezone(calendar.timezone || privacy.timezone)
      setRules(currentRules)
      setFriends(currentFriends)
      setVisibility(privacy.visibility)
      setExactFor(privacy.exact_for)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить расписание')
    } finally {
      setLoading(false)
    }
  }, [from, to])

  useEffect(() => { void load() }, [load])
  useEffect(() => {
    if (!friendIDs.length) { setFriendDays([]); return }
    getFriendAvailability(selectedDate, friendIDs)
      .then(setFriendDays)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : 'Не удалось загрузить расписание друзей'))
  }, [friendIDs, selectedDate])

  const selectedDay = days.find((day) => day.date === selectedDate) ?? { date: selectedDate, state: 'unknown' as const, intervals: [] }
  const calendarCells = useMemo(() => {
    const first = new Date(month.getFullYear(), month.getMonth(), 1)
    const offset = (first.getDay() + 6) % 7
    const count = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate()
    return [...Array(offset).fill(null), ...Array.from({ length: count }, (_, i) => dateKey(new Date(month.getFullYear(), month.getMonth(), i + 1)))] as (string | null)[]
  }, [month])

  async function mutate(action: () => Promise<unknown>) {
    setSaving(true)
    setError('')
    try { await action(); await load() } catch (err) { setError(err instanceof Error ? err.message : 'Не удалось сохранить изменение') }
    finally { setSaving(false) }
  }

  async function addInterval() {
    await mutate(async () => {
      if (editingIntervalID) {
        await updateAvailabilityInterval(editingIntervalID, { start_time: startTime, end_time: endTime, kind: intervalKind })
        setEditingIntervalID('')
      } else {
        await setAvailabilityDay(selectedDate, 'custom')
        await addAvailabilityInterval(selectedDate, { start_time: startTime, end_time: endTime, kind: intervalKind })
      }
    })
  }

  async function addRule() {
    await mutate(() => addAvailabilityRule({
      day_of_week: ruleDay, start_time: startTime, end_time: endTime, kind: intervalKind,
      starts_on: dateKey(new Date()), ends_on: ruleEndsOn || undefined, timezone,
    }))
  }

  async function savePrivacy() {
    await mutate(() => updateAvailabilityPrivacySettings({ visibility, timezone, exact_for: visibility === 'selected_exact' ? exactFor : [] }))
  }

  return (
    <div className="p-4 space-y-5" style={{ backgroundColor: bgColor, color: textColor }}>
      <div className="flex items-center justify-between">
        <div><h1 className="text-2xl font-bold">Расписание</h1><p className="text-sm capitalize" style={{ color: hintColor }}>{timezone}</p></div>
        <button className="btn-ghost text-sm" onClick={() => { const today = new Date(); setMonth(new Date(today.getFullYear(), today.getMonth(), 1)); setSelectedDate(dateKey(today)) }}>Сегодня</button>
      </div>

      <section className="card p-4" style={{ backgroundColor: secondaryBgColor }}>
        <div className="flex items-center justify-between mb-4">
          <button aria-label="Предыдущий месяц" className="btn-ghost" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))}><ChevronLeft /></button>
          <h2 className="font-semibold capitalize">{monthFormatter.format(month)}</h2>
          <button aria-label="Следующий месяц" className="btn-ghost" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))}><ChevronRight /></button>
        </div>
        <div className="grid grid-cols-7 gap-1 text-center text-xs" style={{ color: hintColor }}>
          {['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'].map((day) => <span key={day} className="py-1">{day}</span>)}
        </div>
        <div className="grid grid-cols-7 gap-1">
          {calendarCells.map((date, index) => {
            const day = days.find((item) => item.date === date)
            const active = date === selectedDate
            const today = date === dateKey(new Date())
            return date ? <button key={date} onClick={() => setSelectedDate(date)} className={`relative h-10 rounded-xl text-sm ${active ? 'bg-primary-500 text-white' : today ? 'ring-1 ring-primary-400' : ''}`} style={!active ? { backgroundColor: day?.state === 'free_all' ? '#dcfce7' : day?.state === 'busy_all' ? '#fee2e2' : undefined } : undefined}>
              {dateFromKey(date).getDate()}{day?.intervals.length ? <span className={`absolute bottom-1 left-1/2 h-1 w-1 -translate-x-1/2 rounded-full ${active ? 'bg-white' : 'bg-primary-500'}`} /> : null}
            </button> : <span key={`empty-${index}`} />
          })}
        </div>
        <div className="mt-3 flex flex-wrap gap-2 text-xs" style={{ color: hintColor }}><span>🟢 Свободен</span><span>🔴 Занят</span><span>— Нет данных</span></div>
      </section>

      <section className="card p-4 space-y-3" style={{ backgroundColor: secondaryBgColor }}>
        <h2 className="font-semibold capitalize">{dayFormatter.format(dateFromKey(selectedDate))}</h2>
        <div className="grid grid-cols-3 gap-2">
          {([['free_all', 'Свободен весь день'], ['busy_all', 'Занят весь день'], ['unknown', 'Нет данных']] as const).map(([state, label]) => <button key={state} disabled={saving} className={`rounded-xl border px-2 py-2 text-xs ${selectedDay.state === state ? 'border-primary-500 text-primary-600' : 'border-gray-200'}`} onClick={() => void mutate(() => setAvailabilityDay(selectedDate, state))}>{label}</button>)}
        </div>
        <div className="space-y-2">
          {selectedDay.intervals.map((interval, index) => <div key={interval.id ?? `${interval.start_time}-${index}`} className="flex items-center justify-between rounded-xl bg-black/5 px-3 py-2 text-sm">
            <span>{interval.start_time}–{interval.end_time} · {interval.kind === 'free' ? 'свободен' : 'занят'}</span>
            {interval.id && <div className="flex gap-3"><button aria-label="Изменить интервал" className="text-primary-500" onClick={() => { setEditingIntervalID(interval.id!); setStartTime(interval.start_time); setEndTime(interval.end_time); setIntervalKind(interval.kind) }}><Pencil size={16} /></button><button aria-label="Удалить интервал" className="text-red-500" onClick={() => void mutate(() => deleteAvailabilityInterval(interval.id!))}><Trash2 size={16} /></button></div>}
          </div>)}
          {selectedDay.state === 'unknown' && !selectedDay.intervals.length && <p className="text-sm" style={{ color: hintColor }}>Для этой даты данных пока нет.</p>}
        </div>
        <div className="flex items-end gap-2">
          <label className="flex-1 text-xs">С<input type="time" className="input mt-1" value={startTime} onChange={(event) => setStartTime(event.target.value)} /></label>
          <label className="flex-1 text-xs">До<input type="time" className="input mt-1" value={endTime} onChange={(event) => setEndTime(event.target.value)} /></label>
          <select aria-label="Тип интервала" className="input h-10 w-24 px-2" value={intervalKind} onChange={(event) => setIntervalKind(event.target.value as 'free'|'busy')}><option value="free">Свободен</option><option value="busy">Занят</option></select>
          <button className="btn-primary h-10 px-3" disabled={saving} onClick={() => void addInterval()}><Plus size={18} />{editingIntervalID?'Сохранить':'Добавить'}</button>
        </div>
        {editingIntervalID&&<button className="text-sm text-primary-500" onClick={()=>setEditingIntervalID('')}>Отменить редактирование</button>}
      </section>

      <section className="card p-4 space-y-3" style={{ backgroundColor: secondaryBgColor }}>
        <h2 className="font-semibold">Повторяющееся расписание</h2>
        <div className="grid grid-cols-2 gap-2">
          <select className="input flex-1" value={ruleDay} onChange={(event) => setRuleDay(Number(event.target.value))}>{weekdayNames.map((day, index) => <option key={day} value={index}>{day}</option>)}</select>
          <label className="text-xs">Действует до<input type="date" className="input mt-1" value={ruleEndsOn} onChange={(event)=>setRuleEndsOn(event.target.value)}/></label>
        </div>
        <button className="btn-primary w-full justify-center" disabled={saving} onClick={() => void addRule()}><Plus size={18} /> Добавить правило</button>
        <div className="space-y-2">
          {rules.filter((rule) => rule.is_active).map((rule) => <div key={rule.id} className="flex items-center justify-between rounded-xl bg-black/5 px-3 py-2 text-sm">
            <span>{weekdayNames[rule.day_of_week]} · {rule.start_time}–{rule.end_time}{rule.ends_on ? ` · до ${rule.ends_on}` : ''}</span>
            <button aria-label="Отключить правило" className="text-red-500" onClick={() => void mutate(() => deleteAvailabilityRule(rule.id))}><Trash2 size={16} /></button>
          </div>)}
          {!rules.some((rule) => rule.is_active) && <p className="text-sm" style={{ color: hintColor }}>Повторяющихся правил пока нет.</p>}
        </div>
      </section>

      <section className="card p-4 space-y-3" style={{ backgroundColor: secondaryBgColor }}>
        <h2 className="font-semibold">Доступность друзей</h2>
        {friends.length ? <div className="flex gap-2 overflow-x-auto pb-1">{friends.map((friend) => <label key={friend.public_id} className="flex shrink-0 items-center gap-2 rounded-xl bg-black/5 px-3 py-2 text-sm"><input type="checkbox" checked={friendIDs.includes(friend.public_id)} onChange={(event) => setFriendIDs((current) => event.target.checked ? [...current, friend.public_id] : current.filter((id) => id !== friend.public_id))} />{friend.display_name}</label>)}</div> : <p className="text-sm" style={{ color: hintColor }}>Добавьте друзей, чтобы видеть их доступность.</p>}
        {friendDays.map((friend) => <div key={friend.public_id} className="rounded-xl bg-black/5 px-3 py-2 text-sm"><b>{friend.display_name}</b> · {friend.exact ? friend.intervals?.length ? friend.intervals.map((item) => `${item.start_time}–${item.end_time}`).join(', ') : labelState(friend.state) : labelState(friend.state)}</div>)}
      </section>

      <section className="card p-4 space-y-3" style={{ backgroundColor: secondaryBgColor }}>
        <h2 className="font-semibold">Приватность расписания</h2>
        <select className="input" value={visibility} onChange={(event) => setVisibility(event.target.value as typeof visibility)}>
          <option value="friends_exact">Друзья видят точные интервалы</option><option value="selected_exact">Точные интервалы только выбранным</option><option value="busy_only">Остальные видят только статус</option>
        </select>
        {visibility === 'selected_exact' && <div className="flex flex-wrap gap-2">{friends.map((friend) => <label key={friend.public_id} className="flex items-center gap-2 rounded-xl bg-black/5 px-3 py-2 text-sm"><input type="checkbox" checked={exactFor.includes(friend.public_id)} onChange={(event) => setExactFor((current) => event.target.checked ? [...current, friend.public_id] : current.filter((id) => id !== friend.public_id))} />{friend.display_name}</label>)}</div>}
        <label className="block text-xs">Часовой пояс IANA<input className="input mt-1" value={timezone} onChange={(event) => setTimezone(event.target.value)} placeholder="Europe/Moscow" /></label>
        <button className="btn-secondary w-full" disabled={saving} onClick={() => void savePrivacy()}>{saving ? <LoaderCircle className="animate-spin" size={16} /> : null}Сохранить приватность</button>
      </section>

      {error && <p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-700">{error}</p>}
      {loading && <div className="flex items-center justify-center gap-2 text-sm" style={{ color: hintColor }}><LoaderCircle className="animate-spin" size={16} />Загружаю расписание…</div>}
    </div>
  )
}

function labelState(state: FriendAvailability['state']) {
  if (state === 'free' || state === 'free_all') return 'Свободен'
  if (state === 'busy' || state === 'busy_all') return 'Занят'
  return 'Нет данных'
}
