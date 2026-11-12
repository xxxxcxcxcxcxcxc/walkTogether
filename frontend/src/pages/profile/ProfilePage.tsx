import { useEffect, useState } from 'react'
import { Bell, Globe, LoaderCircle, LogOut, Lock, Save, ShieldCheck } from 'lucide-react'
import { useTelegramTheme } from '../../hooks/useTelegramTheme'
import {
  getAvailabilityPrivacySettings, getFriendPrivacySettings, getFriends, getNotificationSettings, getWalks,
  logoutFromApp, updateAvailabilityPrivacySettings, updateDisplayName, updateFriendPrivacySettings,
  updateNotificationSettings, type AuthUser, type FriendPrivacySettings, type NotificationSettings,
} from '../../services/auth'

type AvailabilityPrivacy={visibility:'friends_exact'|'selected_exact'|'busy_only';timezone:string;exact_for:string[]}

export function ProfilePage({user,onLogout}:{user:AuthUser;onLogout:()=>void}){
  const {bgColor,textColor,hintColor,secondaryBgColor}=useTelegramTheme()
  const [name,setName]=useState(user.display_name)
  const [editing,setEditing]=useState(false)
  const [friends,setFriends]=useState(0)
  const [walks,setWalks]=useState(0)
  const [completed,setCompleted]=useState(0)
  const [friendPrivacy,setFriendPrivacy]=useState<FriendPrivacySettings>({visibility:'friends',friend_notifications_enabled:true})
  const [availabilityPrivacy,setAvailabilityPrivacy]=useState<AvailabilityPrivacy>({visibility:'friends_exact',timezone:Intl.DateTimeFormat().resolvedOptions().timeZone||'UTC',exact_for:[]})
  const [notifications,setNotifications]=useState<NotificationSettings>({friend_notifications_enabled:true,walk_notifications_enabled:true})
  const [error,setError]=useState('')
  const [busy,setBusy]=useState(false)
  useEffect(()=>{
    Promise.all([getFriends(),getWalks(),getFriendPrivacySettings(),getAvailabilityPrivacySettings(),getNotificationSettings()]).then(([friendList,walkList,profilePrivacy,calendarPrivacy,notificationSettings])=>{
      setFriends(friendList.length);setWalks(walkList.filter((walk)=>walk.is_organizer).length);setCompleted(walkList.filter((walk)=>walk.status==='completed').length);setFriendPrivacy(profilePrivacy);setAvailabilityPrivacy(calendarPrivacy);setNotifications(notificationSettings)
    }).catch((err:unknown)=>setError(err instanceof Error?err.message:'Не удалось загрузить настройки профиля'))
  },[])
  async function save(action:()=>Promise<unknown>){setBusy(true);setError('');try{await action()}catch(err){setError(err instanceof Error?err.message:'Не удалось сохранить настройки')}finally{setBusy(false)}}
  async function saveName(){await save(async()=>{await updateDisplayName(name);setEditing(false)})}
  async function logout(){setBusy(true);const ok=await logoutFromApp();setBusy(false);if(ok)onLogout();else setError('Не удалось завершить сессию. Повторите попытку.')}
  const initials=name.trim().split(/\s+/).slice(0,2).map((part)=>part[0]).join('').toUpperCase()||'•'

  return <div className="p-4 space-y-5" style={{backgroundColor:bgColor,color:textColor}}>
    <div className="flex items-center justify-between"><h1 className="text-2xl font-bold">Профиль</h1><button disabled={busy} onClick={()=>void logout()} className="btn-ghost text-red-500"><LogOut size={17}/>Выход</button></div>
    <section className="card flex items-center gap-3 p-4" style={{backgroundColor:secondaryBgColor}}>
      {user.avatar_url?<img src={user.avatar_url} alt="Аватар" className="h-14 w-14 rounded-2xl object-cover"/>:<div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-500 text-xl font-bold text-white">{initials}</div>}
      <div className="min-w-0 flex-1">{editing?<input className="input" value={name} maxLength={80} onChange={(event)=>setName(event.target.value)}/>:<h2 className="truncate text-lg font-semibold">{name}</h2>}<p className="text-sm" style={{color:hintColor}}>{user.username?`@${user.username}`:'Имя профиля'}</p></div>
      {editing?<button disabled={busy} className="btn-primary px-3" onClick={()=>void saveName()}><Save size={16}/></button>:<button className="btn-secondary text-sm" onClick={()=>setEditing(true)}>Изменить имя</button>}
    </section>
    <div className="grid grid-cols-3 gap-2"><Stat label="Друзей" value={friends}/><Stat label="Прогулок" value={walks}/><Stat label="Завершено" value={completed}/></div>

    <section className="card space-y-3 p-4" style={{backgroundColor:secondaryBgColor}}><h2 className="flex items-center gap-2 font-semibold"><Bell size={18}/>Уведомления Telegram</h2>
      <Toggle label="Запросы в друзья" checked={notifications.friend_notifications_enabled} onChange={(value)=>{const next={...notifications,friend_notifications_enabled:value};setNotifications(next);setFriendPrivacy((current)=>({...current,friend_notifications_enabled:value}));void save(()=>updateNotificationSettings(next))}}/>
      <Toggle label="Приглашения и изменения прогулок" checked={notifications.walk_notifications_enabled} onChange={(value)=>{const next={...notifications,walk_notifications_enabled:value};setNotifications(next);void save(()=>updateNotificationSettings(next))}}/>
    </section>

    <section className="card space-y-3 p-4" style={{backgroundColor:secondaryBgColor}}><h2 className="flex items-center gap-2 font-semibold"><Lock size={18}/>Приватность профиля</h2><label className="block text-sm">Кто может найти профиль<select className="input mt-1" value={friendPrivacy.visibility} onChange={(event)=>setFriendPrivacy({...friendPrivacy,visibility:event.target.value as FriendPrivacySettings['visibility']})}><option value="all">Все пользователи</option><option value="friends">Только друзья</option><option value="hidden">Никто</option></select></label><button disabled={busy} className="btn-secondary w-full" onClick={()=>void save(()=>updateFriendPrivacySettings(friendPrivacy))}>Сохранить приватность профиля</button></section>

    <section className="card space-y-3 p-4" style={{backgroundColor:secondaryBgColor}}><h2 className="flex items-center gap-2 font-semibold"><Globe size={18}/>Видимость расписания</h2><label className="block text-sm">Часовой пояс<input className="input mt-1" value={availabilityPrivacy.timezone} onChange={(event)=>setAvailabilityPrivacy({...availabilityPrivacy,timezone:event.target.value})} placeholder="Europe/Moscow"/></label><label className="block text-sm">Доступ друзей<select className="input mt-1" value={availabilityPrivacy.visibility} onChange={(event)=>setAvailabilityPrivacy({...availabilityPrivacy,visibility:event.target.value as AvailabilityPrivacy['visibility']})}><option value="friends_exact">Все друзья видят точные интервалы</option><option value="selected_exact">Точные интервалы выбранным друзьям</option><option value="busy_only">Остальным только статус</option></select></label>{availabilityPrivacy.visibility==='selected_exact'&&<p className="text-xs" style={{color:hintColor}}>Выберите друзей на вкладке «Расписание».</p>}<button disabled={busy} className="btn-secondary w-full" onClick={()=>void save(()=>updateAvailabilityPrivacySettings(availabilityPrivacy))}>Сохранить расписание</button></section>

    {user.is_admin&&<div className="card flex items-center gap-2 p-3 text-sm" style={{backgroundColor:secondaryBgColor}}><ShieldCheck className="text-primary-500" size={18}/>Администратор</div>}
    {error&&<p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-700">{error}</p>}
    {busy&&<div className="flex justify-center"><LoaderCircle className="animate-spin" size={18}/></div>}
  </div>
}

function Stat({label,value}:{label:string;value:number}){return <div className="card p-3 text-center"><div className="text-2xl font-bold text-primary-500">{value}</div><div className="text-xs text-gray-500">{label}</div></div>}
function Toggle({label,checked,onChange}:{label:string;checked:boolean;onChange:(value:boolean)=>void}){return <label className="flex items-center justify-between gap-3 text-sm"><span>{label}</span><input type="checkbox" checked={checked} onChange={(event)=>onChange(event.target.checked)}/></label>}
