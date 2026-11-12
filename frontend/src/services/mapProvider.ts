import { searchPlaces, type PlaceSuggestion } from './auth'

export type MapPoint = { latitude: number; longitude: number }

export interface MapProvider {
  readonly id: string
  readonly tileUrl: string
  readonly attribution: string
  search(query: string): Promise<PlaceSuggestion[]>
}

export const mapProvider: MapProvider = {
  id: 'openstreetmap',
  tileUrl: import.meta.env.VITE_MAP_TILE_URL || 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
  attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap contributors</a>',
  search: searchPlaces,
}
