import { useEffect, useState } from 'react'
import { AlertCircle, Calendar, CheckCircle, Clock, LoaderCircle, MapPin, Plus, UserCheck } from 'lucide-react'
import { useTelegramTheme } from '../../hooks/useTelegramTheme'
import { getFriendAvailability, getFriendRequests, getFriends, getWalks, respondToWalk, type AuthUser, type FriendAvailability, type FriendRequestItem, type WalkEvent } from '../../services/auth'

const now = new Date()
const today = `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,'0')}-${String(now.getDate()).padStart(2,'0')}`
const dateFmt = new Intl.DateTimeFormat('ru-RU',{dateStyle:'medium',timeStyle:'short'})

export function HomePage({ user, onOpenWalks, onOpenSchedule }: { user: AuthUser; onOpenWalks: () => void; onOpenSchedule: () => void }) {
  const { bgColor, textColor, hintColor, secondaryBgColor } = useTelegramTheme()
  const [walks,setWalks]=useState<WalkEvent[]>([])
  const [requests,setRequests]=useState<FriendRequestItem[]>([])
  const [friends,setFriends]=useState<FriendAvailability[]>([])
  const [friendCount,setFriendCount]=useState(0)
  const [error,setError]=useState('')
  const [loading,setLoading]=useState(true)
  async function respond(id:string,action:'accept'|'reject'){
    try{await respondToWalk(id,action);setWalks(await getWalks())}
    catch(err){setError(err instanceof Error?err.message:'Не удалось отправить ответ')}
  }
  useEffect(()=>{
    let alive=true
    async function load(){
      try{
        const [allWalks,incoming,friendList]=await Promise.all([getWalks(),getFriendRequests('incoming'),getFriends()])
        const availability=friendList.length?await getFriendAvailability(today,friendList.map((friend)=>friend.public_id)):[]
        if(!alive)return
        setWalks(allWalks);setRequests(incoming);setFriendCount(friendList.length);setFriends(availability.filter((friend)=>friend.state==='free'||friend.state==='free_all'))
      }catch(err){if(alive)setError(err instanceof Error?err.message:'Не удалось загрузить главную страницу')}
      finally{if(alive)setLoading(false)}
    }
    void load();return()=>{alive=false}
  },[])
  const nextWalk=walks.filter((walk)=>walk.status==='confirmed'&&new Date(walk.scheduled_start).getTime()>=Date.now()).sort((a,b)=>a.scheduled_start.localeCompare(b.scheduled_start))[0]
  const incoming=walks.filter((walk)=>!walk.is_organizer&&['pending','rescheduled'].includes(walk.status))
  const recent=walks.flatMap((walk)=>walk.history.map((event)=>({id:`${walk.id}-${event.created_at}-${event.event_type}`,title:historyLabel(event.event_type),description:`${walk.title||'Прогулка'}${event.actor_name?` · ${event.actor_name}`:''}`,created:new Date(event.created_at)}))).sort((a,b)=>b.created.getTime()-a.created.getTime()).slice(0,4)
  const initials=user.display_name.trim().split(/\s+/).slice(0,2).map((part)=>part[0]).join('').toUpperCase()||'•'

  return <div className="p-4 space-y-5" style={{backgroundColor:bgColor,color:textColor}}>
    <header className="flex items-center justify-between"><div><p className="text-sm" style={{color:hintColor}}>{new Intl.DateTimeFormat('ru-RU',{dateStyle:'full'}).format(new Date())}</p><h1 className="mt-1 text-2xl font-bold">Привет, {user.display_name.split(' ')[0]}!</h1></div><div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-500 text-xl font-bold text-white">{initials}</div></header>
    <button className="btn-primary w-full justify-center py-3" onClick={onOpenWalks}><Plus size={19}/>Создать прогулку</button>
    {error&&<p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-700">{error}</p>}
    {loading?<div className="flex justify-center py-8"><LoaderCircle className="animate-spin"/></div>:<>
      {nextWalk&&<button onClick={onOpenWalks} className="card w-full p-4 text-left" style={{backgroundColor:secondaryBgColor}}><div className="flex items-center gap-2 text-sm" style={{color:hintColor}}><Calendar size={16}/>{dateFmt.format(new Date(nextWalk.scheduled_start))}<Clock size={15}/>{new Intl.DateTimeFormat('ru-RU',{timeStyle:'short'}).format(new Date(nextWalk.scheduled_end))}</div><h2 className="mt-2 font-semibold">{nextWalk.title||'Прогулка'}</h2>{nextWalk.location_name&&<p className="mt-1 flex items-center gap-1 text-sm" style={{color:hintColor}}><MapPin size={14}/>{nextWalk.location_name}</p>}</button>}
      <div className="grid grid-cols-3 gap-2"><Stat value={String(friends.filter((friend)=>friend.state==='free'||friend.state==='free_all').length)} label="Свободны сегодня"/><Stat value={String(incoming.length)} label="Приглашения"/><Stat value={String(friendCount)} label="Друзья"/></div>
      {!!incoming.length&&<section><h2 className="mb-2 font-semibold">Приглашения на прогулку</h2><div className="space-y-2">{incoming.slice(0,3).map((walk)=><div key={walk.id} className="card p-3" style={{backgroundColor:secondaryBgColor}}><button onClick={onOpenWalks} className="w-full text-left"><p className="font-medium">{walk.title||'Прогулка'}</p><p className="mt-1 text-xs" style={{color:hintColor}}>{dateFmt.format(new Date(walk.scheduled_start))} · {walk.organizer_name}</p></button><div className="mt-3 flex gap-2"><button className="btn-outline flex-1 text-sm" onClick={()=>void respond(walk.id,'reject')}>Отклонить</button><button className="btn-primary flex-1 text-sm" onClick={()=>void respond(walk.id,'accept')}>Принять</button></div></div>)}</div></section>}
      <section><div className="mb-2 flex items-center justify-between"><h2 className="font-semibold">Кто свободен сегодня</h2><button className="text-sm text-primary-500" onClick={onOpenSchedule}>Расписание</button></div>{friends.length?<div className="flex gap-2 overflow-x-auto">{friends.map((friend)=><div key={friend.public_id} className="card min-w-28 p-3 text-center" style={{backgroundColor:secondaryBgColor}}><div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl bg-green-100 font-semibold text-green-700">{friend.display_name[0]}</div><p className="mt-1 max-w-24 truncate text-xs">{friend.display_name}</p><p className="text-[11px]" style={{color:hintColor}}>{friend.exact&&friend.intervals?.length?friend.intervals.map((interval)=>`${interval.start_time}–${interval.end_time}`).join(', '):'Свободен'}</p></div>)}</div>:<p className="card p-4 text-sm" style={{backgroundColor:secondaryBgColor,color:hintColor}}>Друзья, которые разрешили показывать доступность, пока не отметили, что свободны.</p>}</section>
      {requests.length>0&&<section><h2 className="mb-2 font-semibold">Запросы в друзья</h2>{requests.slice(0,3).map((request)=><div key={request.id} className="card mb-2 flex items-center justify-between p-3" style={{backgroundColor:secondaryBgColor}}><span className="text-sm">{request.user.display_name}</span><span className="badge badge-warning">Ожидает ответа</span></div>)}</section>}
      <section><h2 className="mb-2 font-semibold">Последние события</h2>{recent.length?<div className="space-y-2">{recent.map((item)=><div key={item.id} className="card flex items-center gap-3 p-3" style={{backgroundColor:secondaryBgColor}}><ActivityIcon title={item.title}/><div className="min-w-0 flex-1"><p className="text-sm font-medium">{item.title}</p><p className="truncate text-xs" style={{color:hintColor}}>{item.description}</p></div><time className="text-[11px]" style={{color:hintColor}}>{new Intl.DateTimeFormat('ru-RU',{dateStyle:'short',timeStyle:'short'}).format(item.created)}</time></div>)}</div>:<p className="card p-4 text-sm" style={{backgroundColor:secondaryBgColor,color:hintColor}}>Новых событий пока нет.</p>}</section>
      <div className="grid grid-cols-2 gap-3"><button onClick={onOpenWalks} className="card p-4 text-center" style={{backgroundColor:secondaryBgColor}}><Plus className="mx-auto text-primary-500"/><span className="mt-2 block text-sm font-medium">Создать прогулку</span></button><button onClick={onOpenSchedule} className="card p-4 text-center" style={{backgroundColor:secondaryBgColor}}><Calendar className="mx-auto text-green-500"/><span className="mt-2 block text-sm font-medium">Моё расписание</span></button></div>
    </>}
  </div>
}

function Stat({value,label}:{value:string;label:string}){return <div className="card p-3 text-center"><div className="text-2xl font-bold text-primary-500">{value}</div><div className="text-[11px] leading-4 text-gray-500">{label}</div></div>}
function ActivityIcon({title}:{title:string}){const Icon=title.includes('прин')||title.includes('подтверж')?CheckCircle:title.includes('запрос')?UserCheck:AlertCircle;return <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary-100 text-primary-500"><Icon size={18}/></div>}
function historyLabel(type:string){return ({created:'Создана прогулка',participant_accepted:'Приглашение принято',participant_rejected:'Приглашение отклонено',confirmed:'Прогулка подтверждена',rescheduled:'Изменено время',cancelled:'Прогулка отменена',completed:'Прогулка завершена'} as Record<string,string>)[type]??type}
