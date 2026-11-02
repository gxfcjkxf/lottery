import {createConfiguredBusinessClient} from '@lottery/shared'

export interface UserGame { id: string; name: string; kind: 'configured' }
export function gamePath(game: UserGame) { return `/configured/${encodeURIComponent(game.id)}` }
export async function loadUserGames(brandCode?: string): Promise<UserGame[]> {
  const configured = await createConfiguredBusinessClient('user', '', brandCode).games()
  const items: UserGame[] = configured.games.map(g => ({ id: g.id, name: g.name, kind: 'configured' }))
  return items
}
export function rememberGame(storage: Pick<Storage,'setItem'>,brandId:string,game:UserGame) { storage.setItem(`lottery:last-game:${brandId}`,JSON.stringify({id:game.id,kind:game.kind})) }
export function recalledGame(storage: Pick<Storage,'getItem'>,brandId:string,games:UserGame[]): UserGame | null {
  const raw=storage.getItem(`lottery:last-game:${brandId}`)
  if(raw===null)return null
  const value=JSON.parse(raw) as {id?:unknown;kind?:unknown}
  return games.find(g=>g.id===value.id&&g.kind===value.kind)??null
}
