import type { Initial, Profile } from './types';
interface Backend {
 Initial():Promise<Initial>;SetDraft(p:Profile,remember:boolean):Promise<void>;SaveProfile(p:Profile,remember:boolean):Promise<void>;Connect():Promise<void>;Disconnect():Promise<void>;
 SetSystemProxy(enabled:boolean):Promise<void>;CheckProxy(p:Profile):Promise<void>;CheckInternet():Promise<void>;CheckUpdates():Promise<void>;OpenUpdate():Promise<void>;ExportLog():Promise<void>;HideToTray():Promise<void>;Quit():Promise<void>;
}
declare global{interface Window{go?:{main?:{App?:Backend}}}}
export function backend():Backend {const b=window.go?.main?.App;if(!b)throw new Error('Запустите CSQTT.exe или wails dev.');return b;}
