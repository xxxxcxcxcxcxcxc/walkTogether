import { useEffect, useRef, useState } from 'react'
import L, { type Map as LeafletMap, type Marker } from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { LoaderCircle, MapPin, Search } from 'lucide-react'
import { mapProvider } from '../services/mapProvider'
import type { PlaceSuggestion } from '../services/auth'

const markerIcon = L.icon({
  iconUrl: new URL('leaflet/dist/images/marker-icon.png', import.meta.url).href,
  iconRetinaUrl: new URL('leaflet/dist/images/marker-icon-2x.png', import.meta.url).href,
  shadowUrl: new URL('leaflet/dist/images/marker-shadow.png', import.meta.url).href,
  iconSize: [25, 41], iconAnchor: [12, 41],
})

export function PlacePicker({ value, onChange }: { value: PlaceSuggestion | null; onChange: (place: PlaceSuggestion) => void }) {
  const mapElement = useRef<HTMLDivElement>(null)
  const mapRef = useRef<LeafletMap | null>(null)
  const markerRef = useRef<Marker | null>(null)
  const onChangeRef = useRef(onChange)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<PlaceSuggestion[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  onChangeRef.current = onChange

  useEffect(() => {
    if (!mapElement.current) return
    const map = L.map(mapElement.current, { zoomControl: false }).setView([55.751244, 37.618423], 10)
    L.tileLayer(mapProvider.tileUrl, { maxZoom: 19, attribution: mapProvider.attribution }).addTo(map)
    L.control.zoom({ position: 'bottomright' }).addTo(map)
    map.on('click', (event) => onChangeRef.current({ name: 'Выбранная точка', address: '', latitude: event.latlng.lat, longitude: event.latlng.lng }))
    mapRef.current = map
    const resize = window.setTimeout(() => map.invalidateSize(), 150)
    return () => { window.clearTimeout(resize); map.remove(); mapRef.current = null }
  }, [])

  useEffect(() => {
    const map = mapRef.current
    if (!map || !value) return
    markerRef.current?.remove()
    markerRef.current = L.marker([value.latitude, value.longitude], { icon: markerIcon }).addTo(map)
    map.setView([value.latitude, value.longitude], Math.max(map.getZoom(), 14))
  }, [value])

  async function search() {
    setBusy(true); setError('')
    try { setResults(await mapProvider.search(query)) }
    catch (err) { setError(err instanceof Error ? err.message : 'Адрес не найден') }
    finally { setBusy(false) }
  }

  function locate() {
    if (!navigator.geolocation) { setError('Геолокация недоступна в этом браузере'); return }
    setError('')
    navigator.geolocation.getCurrentPosition(({ coords }) => onChange({ name: 'Моё место', address: '', latitude: coords.latitude, longitude: coords.longitude }), () => setError('Не удалось получить местоположение. Проверьте разрешение Telegram.'))
  }

  return <div className="space-y-2">
    <div className="flex gap-2"><input className="input min-w-0 flex-1" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Найти адрес или место" onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void search() } }} /><button type="button" disabled={busy} className="btn-secondary px-3" onClick={() => void search()}>{busy ? <LoaderCircle className="animate-spin" size={17} /> : <Search size={17} />}</button></div>
    {results.length > 0 && <div className="max-h-28 space-y-1 overflow-y-auto rounded-xl border p-2">{results.map((place, index) => <button type="button" key={`${place.latitude}-${place.longitude}-${index}`} onClick={() => { onChange(place); setResults([]); setQuery(place.name) }} className="block w-full rounded-lg p-2 text-left text-xs hover:bg-primary-50"><span className="block font-medium">{place.name}</span><span className="text-gray-500">{place.address}</span></button>)}</div>}
    <div ref={mapElement} className="h-56 w-full rounded-xl" aria-label="Карта выбора точки" />
    <div className="flex items-center justify-between gap-2"><p className="min-w-0 truncate text-xs text-gray-500">{value ? `${value.latitude.toFixed(5)}, ${value.longitude.toFixed(5)}` : 'Нажмите на карту, чтобы выбрать точку'}</p><button type="button" className="btn-ghost shrink-0 text-sm" onClick={locate}><MapPin size={16} /> Моё местоположение</button></div>
    <p className="text-[10px] text-gray-500" dangerouslySetInnerHTML={{ __html: mapProvider.attribution }} />
    {error && <p role="alert" className="text-xs text-red-600">{error}</p>}
  </div>
}
