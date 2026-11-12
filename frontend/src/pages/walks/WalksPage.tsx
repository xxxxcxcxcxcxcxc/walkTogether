import { useCallback, useEffect, useState } from 'react'
import { ArrowLeft, Calendar, Check, Clock, LoaderCircle, MapPin, Plus, X } from 'lucide-react'
import { useTelegramTheme } from '../../hooks/useTelegramTheme'
import { PlacePicker } from '../../components/PlacePicker'
import { cancelWalk, completeWalk, createWalk, deleteFavoritePlace, downloadWalkICS, findWalkSlots, getFavoritePlaces, getFriends, getWalk, getWalkCalendar, getWalks, respondToWalk, rescheduleWalk, saveFavoritePlace, type FavoritePlace, type FriendProfile, type PlaceSuggestion, type WalkEvent, type WalkTimeSlot } from '../../services/auth'

const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
const dateTimeValue = (date: Date) => `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}T${String(date.getHours()).padStart(2,'0')}:${String(date.getMinutes()).padStart(2,'0')}`
const dateLabel = (value: string) => new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short', timeZone }).format(new Date(value))
const statusLabels: Record<WalkEvent['status'], string> = { draft:'Черновик', pending:'Ждёт ответа', confirmed:'Подтверждена', rejected:'Отклонена', cancelled:'Отменена', expired:'Истекла', completed:'Завершена', rescheduled:'Нужно подтвердить новое время' }

export function WalksPage() {
  const { bgColor, textColor, hintColor, secondaryBgColor } = useTelegramTheme()
  const [walks, setWalks] = useState<WalkEvent[]>([])
  const [friends, setFriends] = useState<FriendProfile[]>([])
  const [selected, setSelected] = useState<WalkEvent | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [filter, setFilter] = useState('all')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [location, setLocation] = useState('')
  const [place, setPlace] = useState<PlaceSuggestion | null>(null)
  const [favoritePlaces, setFavoritePlaces] = useState<FavoritePlace[]>([])
  const [favoriteName, setFavoriteName] = useState('')
  const [favoriteID, setFavoriteID] = useState('')
  const [start, setStart] = useState(() => dateTimeValue(new Date(Date.now() + 86400000)))
  const [end, setEnd] = useState(() => dateTimeValue(new Date(Date.now() + 90000000)))
  const [inviteIDs, setInviteIDs] = useState<string[]>([])
  const [suggestions, setSuggestions] = useState<WalkTimeSlot[]>([])
  const [findingSlots, setFindingSlots] = useState(false)
  const [rescheduleOpen, setRescheduleOpen] = useState(false)
  const [rescheduleStart, setRescheduleStart] = useState('')
  const [rescheduleEnd, setRescheduleEnd] = useState('')
  const [rescheduleLocation, setRescheduleLocation] = useState('')
  const [reschedulePlace, setReschedulePlace] = useState<PlaceSuggestion | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const reload = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [items, friendList, savedPlaces] = await Promise.all([getWalks(), getFriends(), getFavoritePlaces()])
      setWalks(items)
      setFriends(friendList)
      setFavoritePlaces(savedPlaces)
    } catch (err) { setError(err instanceof Error ? err.message : 'Не удалось загрузить прогулки') }
    finally { setLoading(false) }
  }, [])
  useEffect(() => { void reload() }, [reload])

  async function run(action: () => Promise<unknown>) {
    setBusy(true); setError('')
    try {
      await action()
      await reload()
      if (selected) setSelected(await getWalk(selected.id))
    }
    catch (err) { setError(err instanceof Error ? err.message : 'Не удалось выполнить действие') }
    finally { setBusy(false) }
  }

  async function submitWalk(event: React.FormEvent) {
    event.preventDefault()
    if (!inviteIDs.length) { setError('Выберите хотя бы одного друга'); return }
    await run(async () => {
      const created = await createWalk({ title: title.trim(), description: description.trim(), scheduled_start: new Date(start).toISOString(), scheduled_end: new Date(end).toISOString(), timezone: timeZone, location_name: location.trim(), latitude: place?.latitude, longitude: place?.longitude, participants: inviteIDs })
      setSelected(created); setCreateOpen(false); setTitle(''); setDescription(''); setLocation(''); setPlace(null); setInviteIDs([])
    })
  }

  async function suggestTimes() {
    if (!inviteIDs.length) { setError('Сначала выберите друзей'); return }
    const duration = Math.round((new Date(end).getTime() - new Date(start).getTime()) / 60000)
    if (duration < 15) { setError('Продолжительность должна быть не меньше 15 минут'); return }
    setFindingSlots(true); setError('')
    try {
      const options = await findWalkSlots(start.slice(0, 10), duration, inviteIDs)
      setSuggestions(options)
      if (!options.length) setError('Общих свободных интервалов не найдено. Выберите время вручную.')
    } catch (err) { setSuggestions([]); setError(err instanceof Error ? err.message : 'Не удалось рассчитать время') }
    finally { setFindingSlots(false) }
  }

  async function keepPlace() {
    if (!place) { setError('Сначала выберите место на карте'); return }
    setBusy(true); setError('')
    try {
      const saved = await saveFavoritePlace({ ...place, name: favoriteName.trim() || place.name || 'Любимое место' })
      setFavoritePlaces((current) => [saved, ...current.filter((item) => item.id !== saved.id)])
      setFavoriteName('')
    } catch (err) { setError(err instanceof Error ? err.message : 'Не удалось сохранить место') }
    finally { setBusy(false) }
  }

  function choosePlace(next: PlaceSuggestion | null) {
    setPlace(next)
    if (next) setLocation(next.address || next.name || `${next.latitude.toFixed(5)}, ${next.longitude.toFixed(5)}`)
  }

  function beginReschedule(walk: WalkEvent) {
    setRescheduleStart(dateTimeValue(new Date(new Date(walk.scheduled_start).getTime() + 86400000)))
    setRescheduleEnd(dateTimeValue(new Date(new Date(walk.scheduled_end).getTime() + 86400000)))
    setRescheduleLocation(walk.location_name || '')
    setReschedulePlace(walk.latitude != null && walk.longitude != null ? { name: walk.location_name || 'Выбранная точка', address: walk.location_name || '', latitude: walk.latitude, longitude: walk.longitude } : null)
    setRescheduleOpen(true)
  }

  async function submitReschedule(event: React.FormEvent) {
    event.preventDefault()
    if (!selected) return
    await run(() => rescheduleWalk(selected.id, new Date(rescheduleStart).toISOString(), new Date(rescheduleEnd).toISOString(), rescheduleLocation, reschedulePlace?.latitude, reschedulePlace?.longitude))
    setRescheduleOpen(false)
  }

  const visibleWalks = walks.filter((walk) => filter === 'all' || (filter === 'upcoming' ? ['confirmed','pending','rescheduled'].includes(walk.status) : filter === 'pending' ? ['pending','rescheduled'].includes(walk.status) : filter === 'history' ? ['completed','cancelled','rejected','expired'].includes(walk.status) : walk.status === filter))
  const grouped = [
    ['Ожидают ответа', visibleWalks.filter((walk) => ['pending','rescheduled'].includes(walk.status))],
    ['Предстоящие', visibleWalks.filter((walk) => walk.status === 'confirmed')],
    ['История', visibleWalks.filter((walk) => ['completed','cancelled','rejected','expired'].includes(walk.status))],
    ['Другие', visibleWalks.filter((walk) => ['draft'].includes(walk.status))],
  ] as const

  return <div className="p-4 space-y-5" style={{ backgroundColor: bgColor, color: textColor }}>
    <div className="flex items-center justify-between"><div><h1 className="text-2xl font-bold">Прогулки</h1><p className="text-sm" style={{ color: hintColor }}>Встречи с друзьями</p></div><button className="btn-primary" onClick={() => setCreateOpen(true)}><Plus size={18} /> Создать</button></div>
    <div className="flex gap-2 overflow-x-auto">{[['all','Все'],['upcoming','Предстоящие'],['pending','Ожидают ответа'],['history','История']].map(([value,label])=><button key={value} onClick={()=>setFilter(value)} className={`rounded-xl px-3 py-2 text-sm whitespace-nowrap ${filter===value?'bg-primary-500 text-white':'bg-black/5'}`}>{label}</button>)}</div>
    {error && <p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-700">{error}</p>}
    {loading ? <div className="flex justify-center py-8"><LoaderCircle className="animate-spin" /></div> : visibleWalks.length === 0 ? <div className="card p-8 text-center" style={{ backgroundColor: secondaryBgColor }}><Calendar className="mx-auto mb-3" size={32} style={{ color: hintColor }} /><h2 className="font-semibold">Пока прогулок нет</h2><p className="mt-1 text-sm" style={{ color: hintColor }}>Создайте прогулку и пригласите друзей.</p></div> : grouped.map(([heading,items])=>items.length? <section key={heading}><h2 className="mb-2 font-semibold">{heading}</h2><div className="space-y-3">{items.map((walk)=><button key={walk.id} onClick={()=>setSelected(walk)} className="card w-full p-4 text-left" style={{ backgroundColor: secondaryBgColor }}><div className="flex justify-between gap-3"><div className="min-w-0"><h3 className="truncate font-semibold">{walk.title||'Прогулка'}</h3><p className="mt-1 flex items-center gap-1 text-sm" style={{color:hintColor}}><Clock size={14}/>{dateLabel(walk.scheduled_start)}</p>{walk.location_name&&<p className="mt-1 flex items-center gap-1 truncate text-sm" style={{color:hintColor}}><MapPin size={14}/>{walk.location_name}</p>}</div><span className={`badge ${walk.status==='confirmed'?'badge-success':walk.status==='rejected'||walk.status==='cancelled'?'badge-danger':'badge-warning'} h-fit`}>{statusLabels[walk.status]}</span></div><p className="mt-2 text-xs" style={{color:hintColor}}>{walk.participants.length} участников · организатор {walk.organizer_name}</p></button>)}</div></section>:null)}

    {createOpen && <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 p-3 sm:items-center">
      <form onSubmit={(event)=>void submitWalk(event)} className="max-h-[92vh] w-full max-w-lg space-y-3 overflow-y-auto rounded-2xl bg-white p-5 text-gray-900 shadow-xl dark:bg-gray-900 dark:text-gray-100">
        <div className="flex items-center justify-between"><h2 className="text-xl font-bold">Новая прогулка</h2><button type="button" onClick={()=>setCreateOpen(false)} aria-label="Закрыть"><X/></button></div>
        <label className="block text-sm">Название (необязательно)<input className="input mt-1" maxLength={120} value={title} onChange={(e)=>setTitle(e.target.value)} placeholder="Вечерняя прогулка"/></label>
        <label className="block text-sm">Комментарий<textarea className="input mt-1" maxLength={2000} value={description} onChange={(e)=>setDescription(e.target.value)}/></label>
        <div className="grid grid-cols-2 gap-2"><label className="text-sm">Начало<input type="datetime-local" required className="input mt-1" value={start} onChange={(e)=>setStart(e.target.value)}/></label><label className="text-sm">Окончание<input type="datetime-local" required className="input mt-1" value={end} onChange={(e)=>setEnd(e.target.value)}/></label></div>
        <fieldset><legend className="mb-1 text-sm font-medium">Кого пригласить</legend><div className="max-h-32 space-y-1 overflow-auto rounded-xl border p-2">{friends.map((friend)=><label key={friend.public_id} className="flex items-center gap-2 py-1 text-sm"><input type="checkbox" checked={inviteIDs.includes(friend.public_id)} onChange={(e)=>{setSuggestions([]);setInviteIDs((current)=>e.target.checked?[...current,friend.public_id]:current.filter((id)=>id!==friend.public_id))}}/>{friend.display_name}</label>)}{!friends.length&&<p className="text-sm text-gray-500">Сначала добавьте друзей.</p>}</div></fieldset>
        <button type="button" className="btn-secondary w-full" disabled={findingSlots} onClick={()=>void suggestTimes()}>{findingSlots?<LoaderCircle className="animate-spin" size={17}/>:<Clock size={17}/>}Подобрать общее свободное время</button>
        {suggestions.length>0&&<div className="max-h-24 space-y-1 overflow-auto rounded-xl border p-2">{suggestions.map((slot)=><button type="button" key={slot.start} onClick={()=>{setStart(dateTimeValue(new Date(slot.start)));setEnd(dateTimeValue(new Date(slot.end)))}} className="block w-full rounded-lg px-2 py-1 text-left text-sm hover:bg-primary-50">{dateLabel(slot.start)} – {new Intl.DateTimeFormat('ru-RU',{timeStyle:'short'}).format(new Date(slot.end))}</button>)}</div>}
        <label className="block text-sm">Место встречи<input className="input mt-1" maxLength={300} value={location} onChange={(e)=>{setLocation(e.target.value);setPlace(null)}} placeholder="Адрес или ориентир"/></label>
        {favoritePlaces.length>0&&<div className="flex gap-2"><select aria-label="Сохранённое место" className="input flex-1" value={favoriteID} onChange={(event)=>{setFavoriteID(event.target.value);const place=favoritePlaces.find((item)=>item.id===event.target.value);if(place)choosePlace(place)}}><option value="">Выбрать сохранённое место</option>{favoritePlaces.map((item)=><option key={item.id} value={item.id}>{item.name}</option>)}</select><button type="button" className="btn-ghost text-red-500" disabled={!favoriteID} title="Удалить выбранное избранное место" onClick={()=>void deleteFavoritePlace(favoriteID).then(()=>{setFavoritePlaces((current)=>current.filter((item)=>item.id!==favoriteID));setFavoriteID('')})}>Удалить</button></div>}
        <PlacePicker value={place} onChange={choosePlace}/>
        {place&&<div className="flex gap-2"><input className="input min-w-0 flex-1" value={favoriteName} onChange={(event)=>setFavoriteName(event.target.value)} placeholder="Название избранного места"/><button type="button" className="btn-secondary shrink-0" disabled={busy} onClick={()=>void keepPlace()}>Сохранить место</button></div>}
        <button disabled={busy} className="btn-primary w-full justify-center">{busy?<LoaderCircle className="animate-spin" size={18}/>:null}Создать приглашения</button>
      </form>
    </div>}

    {selected && <div className="fixed inset-0 z-50 overflow-y-auto bg-black/40 p-3"><div className="mx-auto my-6 max-w-lg rounded-2xl bg-white p-5 text-gray-900 shadow-xl dark:bg-gray-900 dark:text-gray-100"><div className="mb-4 flex items-center justify-between"><button onClick={()=>setSelected(null)} className="btn-ghost"><ArrowLeft size={18}/> Назад</button><span className="badge badge-primary">{statusLabels[selected.status]}</span></div><h2 className="text-xl font-bold">{selected.title||'Прогулка'}</h2>{selected.description&&<p className="mt-2 text-sm">{selected.description}</p>}<p className="mt-3 flex gap-2 text-sm"><Clock size={17}/>{dateLabel(selected.scheduled_start)} – {new Intl.DateTimeFormat('ru-RU',{timeStyle:'short',timeZone}).format(new Date(selected.scheduled_end))}</p>{selected.location_name&&<p className="mt-2 flex gap-2 text-sm"><MapPin size={17}/>{selected.location_name}</p>}{selected.latitude!=null&&selected.longitude!=null&&<a className="mt-2 inline-flex text-sm text-primary-600" target="_blank" rel="noreferrer" href={`https://www.openstreetmap.org/?mlat=${selected.latitude}&mlon=${selected.longitude}#map=16/${selected.latitude}/${selected.longitude}`}>Открыть точку на карте</a>}<p className="mt-4 font-semibold">Участники</p><div className="mt-2 space-y-2">{selected.participants.map((person)=><div key={person.public_id} className="flex justify-between text-sm"><span>{person.display_name}{person.public_id===selected.organizer_public_id?' · организатор':''}</span><span className={person.status==='accepted'?'text-green-600':person.status==='rejected'?'text-red-500':'text-amber-600'}>{person.status==='accepted'?'Принял':person.status==='rejected'?'Отклонил':'Ожидает'}</span></div>)}</div>
      {selected.history.length>0&&<><p className="mt-4 font-semibold">История</p><div className="mt-2 space-y-1 text-xs text-gray-500">{selected.history.map((item,index)=><p key={`${item.event_type}-${index}`}>{dateLabel(item.created_at)} · {historyLabel(item.event_type)}{item.actor_name?` · ${item.actor_name}`:''}</p>)}</div></>}
      {error&&<p role="alert" className="mt-3 text-sm text-red-600">{error}</p>}
      {selected.status==='pending'&&!selected.is_organizer&&<div className="mt-5 flex gap-2"><button disabled={busy} onClick={()=>void run(()=>respondToWalk(selected.id,'reject'))} className="btn-outline flex-1"><X size={16}/>Отклонить</button><button disabled={busy} onClick={()=>void run(()=>respondToWalk(selected.id,'accept'))} className="btn-primary flex-1"><Check size={16}/>Принять</button></div>}
      {selected.status==='rescheduled'&&!selected.is_organizer&&selected.participants.find((p)=>p.status==='pending')&&<button disabled={busy} onClick={()=>void run(()=>respondToWalk(selected.id,'accept'))} className="btn-primary mt-4 w-full justify-center"><Check size={16}/>Подтвердить новое время</button>}
      {selected.is_organizer&&['pending','confirmed','rescheduled'].includes(selected.status)&&<div className="mt-5 space-y-2">{selected.status==='confirmed'&&<button onClick={()=>beginReschedule(selected)} className="btn-secondary w-full">Перенести прогулку</button>}<button disabled={busy} onClick={()=>void run(()=>cancelWalk(selected.id))} className="btn-outline w-full border-red-400 text-red-600">Отменить прогулку</button></div>}
      {selected.status==='confirmed'&&<div className="mt-5 grid grid-cols-1 gap-2"><button onClick={()=>downloadWalkICS(selected.id)} className="btn-secondary w-full"><Calendar size={17}/>Apple Calendar / ICS</button><button onClick={()=>void run(async()=>{const links=await getWalkCalendar(selected.id);window.open(links.google,'_blank','noopener,noreferrer');window.open(links.outlook,'_blank','noopener,noreferrer')})} className="btn-secondary w-full">Google Calendar / Outlook</button></div>}
      {selected.is_organizer&&selected.status==='confirmed'&&new Date(selected.scheduled_end)<new Date()&&<button className="btn-primary mt-3 w-full" onClick={()=>void run(()=>completeWalk(selected.id))}>Отметить завершённой</button>}
    </div></div>}
    {selected&&rescheduleOpen&&<div className="fixed inset-0 z-[60] flex items-end justify-center bg-black/50 p-3 sm:items-center"><form onSubmit={(event)=>void submitReschedule(event)} className="max-h-[92vh] w-full max-w-lg space-y-3 overflow-y-auto rounded-2xl bg-white p-5 text-gray-900 dark:bg-gray-900 dark:text-gray-100"><div className="flex items-center justify-between"><h2 className="text-xl font-bold">Перенести прогулку</h2><button type="button" onClick={()=>setRescheduleOpen(false)}><X/></button></div><div className="grid grid-cols-2 gap-2"><label className="text-sm">Начало<input required type="datetime-local" className="input mt-1" value={rescheduleStart} onChange={(event)=>setRescheduleStart(event.target.value)}/></label><label className="text-sm">Окончание<input required type="datetime-local" className="input mt-1" value={rescheduleEnd} onChange={(event)=>setRescheduleEnd(event.target.value)}/></label></div><label className="block text-sm">Место встречи<input className="input mt-1" value={rescheduleLocation} onChange={(event)=>{setRescheduleLocation(event.target.value);setReschedulePlace(null)}}/></label><PlacePicker value={reschedulePlace} onChange={(next)=>{setReschedulePlace(next);setRescheduleLocation(next.address||next.name)}}/><p className="text-xs text-gray-500">После переноса друзьям нужно подтвердить новое время.</p><button className="btn-primary w-full justify-center" disabled={busy}>Отправить новое предложение</button></form></div>}
  </div>
}

function historyLabel(type:string){return ({created:'создана',participant_accepted:'приглашение принято',participant_rejected:'приглашение отклонено',confirmed:'все участники подтвердили',rescheduled:'изменено время',cancelled:'отменена',completed:'завершена'} as Record<string,string>)[type]??type}
