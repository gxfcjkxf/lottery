import {createConfiguredBusinessClient} from '@lottery/shared'
import { createBettingApi } from './betting-api'

export interface UserGame { id: string; name: string; kind: 'standard' | 'configured' }
export function gamePath(game: UserGame) { return game.kind === 'standard' ? `/games/${encodeURIComponent(game.id)}/bet` : `/${game.kind}/${encodeURIComponent(game.id)}` }
export async function loadUserGames(brandCode?: string): Promise<UserGame[]> {
  const standard = createBettingApi({ brandCode })
  const configured = await createConfiguredBusinessClient('user', '', brandCode).games()
  const items: UserGame[] = configured.games.map(g => ({ id: g.id, name: g.name, kind: 'configured' }))
  for(let offset=0;;offset+=50){const page=await standard.games(50,offset);items.push(...page.items.map(g=>({id:g.id,name:g.name,kind:'standard' as const})));if(page.items.length<50)break}
  return items
}
export function rememberGame(storage: Pick<Storage,'setItem'>,brandId:string,game:UserGame) { storage.setItem(`lottery:last-game:${brandId}`,JSON.stringify({id:game.id,kind:game.kind})) }
export function recalledGame(storage: Pick<Storage,'getItem'>,brandId:string,games:UserGame[]): UserGame | null {
  const raw=storage.getItem(`lottery:last-game:${brandId}`)
  if(raw===null)return null
  const value=JSON.parse(raw) as {id?:unknown;kind?:unknown}
  return games.find(g=>g.id===value.id&&g.kind===value.kind)??null
}
