export interface Profile {
 server:string;password:string;hashes:string;proxy:string;proxy_user:string;proxy_password:string;
 socks_port:string;http_port:string;workers:string;turn:string;obfs:string;srtp:string;generation:string;salt:string;internet_mode:string;system_proxy:string;tun:string;
}
export interface Update {version:string;url:string}
export interface State {status:string;message:string;active:number;upload:number;download:number;ip:string;ready:boolean;logs:{time:string;message:string;level:string}[];revision:number;update:Update|null;updateChecking:boolean;updateMessage:string}
export interface Initial {profile:Profile;state:State;version:string;error:string}
export const defaults:Profile={server:'',password:'',hashes:'',proxy:'10.0.0.1:3128',proxy_user:'',proxy_password:'',socks_port:'1080',http_port:'8080',workers:'6',turn:'',obfs:'unified',srtp:'auto',generation:'0',salt:'',internet_mode:'proxy',system_proxy:'0',tun:'0'};
