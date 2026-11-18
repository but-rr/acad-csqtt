export interface WindowRuntime {
 WindowToggleMaximise():void;WindowIsMaximised():Promise<boolean>;
 EventsOn(name:string,callback:(data:any)=>void):()=>void;
}
declare global{interface Window{runtime?:WindowRuntime}}
export const windowRuntime=()=>window.runtime;
