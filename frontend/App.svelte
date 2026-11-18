<script lang="ts">
 import {onMount} from 'svelte';
 import TitleBar from './components/TitleBar.svelte';
 import Icon from './components/Icon.svelte';
 import Fields from './components/Fields.svelte';
 import UpdateDialog from './components/UpdateDialog.svelte';
 import logo from '../assets/icons/CS_logo.png';
 import {backend} from './api';
 import {defaults,type Profile,type State,type Update} from './types';
 import {version} from './package.json';
 let profile:Profile={...defaults};let remember=true;let page='connection';let loading=true;let action=false;let error='';let saveMessage='';let proxyMessage='';let proxyOK=false;let proxyBusy=false;
 let state:State={status:'offline',message:'Готов к подключению',active:0,upload:0,download:0,ip:'',ready:false,logs:[],revision:0,update:null,updateChecking:false,updateMessage:''};
 let modal:Update|null=null;let dismissed='';let manualUpdate=false;let saveTimer:ReturnType<typeof setTimeout>;let saveRevision=0;let saveChain=Promise.resolve();let logBox:HTMLDivElement;
 $: online=state.status==='online';$: running=state.status!=='offline';$: stopping=state.status==='stopping';$: connecting=['connecting','checking','recovering'].includes(state.status);
 $: statusTitle=({offline:'Отключено',connecting:'Подключение',checking:'Проверка интернета',online:'Подключено',recovering:'Восстановление',stopping:'Отключение',unconfirmed:'Нужна проверка'} as Record<string,string>)[state.status]||state.status;
 $: if(state.update&&state.update.version!==dismissed)modal=state.update;
 $: if(manualUpdate&&!state.updateChecking){manualUpdate=false;if(state.update)modal=state.update;}
 const mb=(value:number)=>(value/1048576).toLocaleString('ru-RU',{maximumFractionDigits:2,minimumFractionDigits:2});
 function accept(next:State){state=next;if(page==='logs'&&logBox&&logBox.scrollHeight-logBox.scrollTop-logBox.clientHeight<70)requestAnimationFrame(()=>logBox.scrollTop=logBox.scrollHeight);}
 onMount(()=>{let disposed=false;let unsubscribe:undefined|(()=>void);
  void backend().Initial().then(data=>{if(disposed)return;profile=data.profile;accept(data.state);error=data.error;loading=false;unsubscribe=window.runtime?.EventsOn('state',accept);}).catch(e=>{error=String(e);loading=false;});
  const unload=()=>{clearTimeout(saveTimer);if(saveRevision)void flush();};window.addEventListener('beforeunload',unload);
  return()=>{disposed=true;unsubscribe?.();clearTimeout(saveTimer);window.removeEventListener('beforeunload',unload);};
 });
 function dirty(){saveRevision++;saveMessage='Сохранение…';void backend().SetDraft({...profile},remember).catch(e=>error=String(e));clearTimeout(saveTimer);saveTimer=setTimeout(()=>void flush().catch(e=>error=String(e)),400);}
 async function flush(){clearTimeout(saveTimer);const revision=saveRevision;const value={...profile};const enabled=remember;
  saveChain=saveChain.catch(()=>{}).then(()=>backend().SaveProfile(value,enabled));
  try{await saveChain;if(saveRevision===revision){saveRevision=0;saveMessage=enabled?'Настройки сохранены':'Сохранение выключено';}}catch(e){saveMessage='Не удалось сохранить';throw e;}
 }
 async function toggle(){if(action||stopping)return;action=true;error='';try{if(running){await backend().Disconnect();state={...state,status:'stopping',message:'Восстанавливаю настройки сети…'};}else{await flush();await backend().Connect();state={...state,status:'connecting',message:'Получаю конфигурацию сервера…'};}}catch(e){error=String(e);if(!running)page='settings';}finally{action=false;}}
 async function mode(key:keyof Profile,value:string){profile[key]=value;dirty();if(key==='system_proxy'){try{await flush();await backend().SetSystemProxy(value==='1');}catch(e){error=String(e);}}}
 async function checkProxy(){proxyBusy=true;proxyMessage='Проверяю CONNECT на 443…';try{await backend().CheckProxy({...profile});proxyOK=true;proxyMessage='Прокси доступен · CONNECT :443 разрешён';}catch(e){proxyOK=false;proxyMessage=String(e);}finally{proxyBusy=false;}}
 async function checkUpdates(){manualUpdate=true;state={...state,updateChecking:true};await flush();await backend().CheckUpdates();}
 async function openUpdate(){try{await backend().OpenUpdate();dismissed=modal?.version||'';modal=null;}catch(e){error=String(e);}}
 function later(){dismissed=modal?.version||state.update?.version||'';modal=null;}
 async function copyLogs(){try{await navigator.clipboard.writeText(state.logs.map(l=>l.time+' '+l.message).join('\n'));}catch{error='Не удалось скопировать журнал. Используйте экспорт.';}}
 async function hide(){try{await flush();await backend().HideToTray();}catch(e){error=String(e);}}
</script>

<TitleBar connected={online} {loading} onWindowError={message=>error=message}/>
<div class="app-shell">
 <nav class="sidebar" aria-label="Основная навигация"><img class="rail-logo" src={logo} alt="CSQTT"/><div class="rail-divider"></div>
 {#each [{id:'connection',name:'Подключение',icon:'monitor'},{id:'settings',name:'Настройки',icon:'settings'},{id:'logs',name:'Журнал',icon:'logs'}] as item}
 <button class:active={page===item.id} class="nav-item" aria-label={item.name} aria-current={page===item.id?'page':undefined} on:click={()=>page=item.id}><Icon name={item.icon} size={23}/><span class="nav-tooltip">{item.name}</span></button>{/each}
 <div class="side-spacer"></div><span class="rail-version">v{version}</span><span class="rail-status" class:online></span>
 </nav>
 <main>
 <header class="page-header"><div><h1>{page==='connection'?'Подключение':page==='settings'?'Настройки':'Журнал'}</h1><p>{page==='connection'?'Интернет через ваш сервер CSQTT.':page==='settings'?'Сервер, корпоративный прокси и параметры туннеля.':'События подключения и диагностика.'}</p></div><span class="status-badge" class:online class:connecting><i></i>{statusTitle}</span></header>
 {#if error}<div class="notice error" role="alert"><Icon name="alert"/>{error}<button aria-label="Закрыть сообщение" on:click={()=>error=''}><Icon name="x"/></button></div>{/if}
 <div class="page connection-page" hidden={page!=='connection'} inert={page!=='connection'}>
 <section class="stats" aria-label="Статистика подключения">{#each [{name:'Активные потоки',value:String(state.active),unit:'',icon:'server'},{name:'Отправлено',value:mb(state.upload),unit:'МБ',icon:'globe'},{name:'Получено',value:mb(state.download),unit:'МБ',icon:'download'}] as metric}<div class="stat"><div><span>{metric.name}</span><Icon name={metric.icon}/></div><strong>{metric.value}<small>{metric.unit}</small></strong></div>{/each}</section>
 <div class="connection-grid"><section class="card connection-card" class:online class:connecting>
 <div class="card-heading"><span>CSQTT</span><span class="tiny">TCP / 443</span></div>
 <div class="power-area"><button class="power-button" class:online class:connecting class:stopping disabled={loading||action||stopping} on:click={toggle} aria-label={stopping?'Отключение':running?'Отключить':'Подключить'}><span class="power-ring"></span><span class="power-face"><Icon name="power" size={49}/><strong>{stopping?'Отключение…':running?'Отключить':'Подключить'}</strong></span></button><h2>{statusTitle}</h2><p aria-live="polite">{state.message}</p></div>
 <div class="mode-row"><label class="check"><input type="checkbox" checked={profile.system_proxy==='1'} on:change={e=>void mode('system_proxy',e.currentTarget.checked?'1':'0')}/><span>Системный прокси</span></label><label class="check"><input type="checkbox" checked={profile.tun==='1'} disabled={running} on:change={e=>void mode('tun',e.currentTarget.checked?'1':'0')}/><span>TUN · весь компьютер</span></label></div>
 <div class="corporate-row"><div><strong>Корпоративный прокси</strong><span>{profile.internet_mode==='proxy'?'HTTP CONNECT с авторизацией':'Прямое подключение через интернет'}</span></div><label class="switch" aria-label="Корпоративный прокси"><input type="checkbox" checked={profile.internet_mode==='proxy'} disabled={running} on:change={e=>void mode('internet_mode',e.currentTarget.checked?'proxy':'direct')}/><span></span></label></div>
 </section>
 <aside class="card session-card"><div class="section-heading"><Icon name="shield"/><h2>Соединение</h2></div><p class="muted">{profile.tun==='1'?'Весь IPv4-трафик через TUN':'Локальные прокси для приложений'}</p><dl><div><dt>Внешний IP</dt><dd class:online>{state.ip||'—'}</dd></div><div><dt>Транспорт</dt><dd>TURN · TCP / 443</dd></div><div><dt>Подключение</dt><dd>{profile.internet_mode==='proxy'?'Через корпоративный прокси':'Прямой интернет'}</dd></div></dl><div class="local-endpoints"><div><span>SOCKS5</span><code>127.0.0.1:{profile.socks_port}</code></div><div><span>HTTP</span><code>127.0.0.1:{profile.http_port}</code></div></div><button class="btn full" disabled={!state.ready||state.status==='checking'} on:click={()=>void backend().CheckInternet()}><Icon name="refresh" size={16}/>Проверить интернет</button><button class="text-button" on:click={()=>page='settings'}>Настройки подключения<Icon name="arrow" size={14}/></button><p class="session-note">{profile.tun==='1'?'Для TUN нужны права администратора. IPv6 временно блокируется.':'Системный прокси применяется после подключения и восстанавливается при выходе.'}</p></aside></div>
 </div>
 <div class="page settings-page" hidden={page!=='settings'} inert={page!=='settings'}>
 <div class="settings-grid"><section class="card settings-card"><h2>Сервер CSQTT</h2><p>Данные подключения с вашего сервера.</p><Fields disabled={running} bind:profile changed={dirty} fields={[{key:'server',label:'Адрес сервера · IP или домен и порт'},{key:'password',label:'Пароль CSQTT',secret:true}]}/><label class="field"><span>Хеши VK или ссылки звонков</span><textarea disabled={running} rows="4" bind:value={profile.hashes} on:input={dirty} spellcheck="false"></textarea><small>По одному на строку · до 6 хешей</small></label></section>
 <section class="card settings-card"><h2>Корпоративный прокси</h2><p>Используется, когда переключатель на главном экране включён.</p><Fields disabled={running} bind:profile changed={dirty} fields={[{key:'proxy',label:'Адрес прокси · IPv4:порт'},{key:'proxy_user',label:'Логин'},{key:'proxy_password',label:'Пароль прокси',secret:true}]}/><button class="btn" disabled={proxyBusy} on:click={checkProxy}><Icon name="refresh" size={16}/>Проверить прокси :443</button>{#if proxyMessage}<p class="feedback" class:success={proxyOK} role="status">{proxyMessage}</p>{/if}</section>
 <section class="card settings-card"><h2>Туннель</h2><Fields disabled={running} bind:profile changed={dirty} fields={[{key:'workers',label:'Количество потоков'},{key:'obfs',label:'Обфускация',options:['unified','audio','video']},{key:'srtp',label:'SRTP',options:['auto','wrap','dtls']},{key:'turn',label:'TURN-хост',hint:'Необязательно · без схемы и порта'}]}/></section>
 <section class="card settings-card"><h2>Порты и сессия</h2><Fields disabled={running} bind:profile changed={dirty} fields={[{key:'socks_port',label:'Локальный SOCKS5'},{key:'http_port',label:'Локальный HTTP'},{key:'generation',label:'Поколение'},{key:'salt',label:'Salt',secret:true}]}/></section>
 <section class="card settings-card"><h2>Сохранение настроек</h2><label class="check"><input type="checkbox" bind:checked={remember} on:change={dirty}/><span>Сохранять рядом с программой</span></label><p>Все настройки сохраняются в <code>profile.json</code>. Пароли и хеши хранятся открытым текстом. Для переноса скопируйте всю папку программы.</p><span class="save-status" role="status">{saveMessage||'Изменения сохраняются автоматически'}</span></section>
 <section class="card settings-card"><h2>Обновления</h2><p>Установлена версия v{version}</p><button class="btn" disabled={state.updateChecking} on:click={()=>void checkUpdates().catch(e=>error=String(e))}><Icon name="refresh" size={16}/>{state.updateChecking?'Проверяю…':'Проверить обновления'}</button><p class="feedback">{state.updateMessage}</p>{#if state.update}<button class="text-button" on:click={()=>modal=state.update}>Посмотреть обновление<Icon name="arrow" size={14}/></button>{/if}</section></div>
 </div>
 <div class="page logs-page" hidden={page!=='logs'} inert={page!=='logs'}><div class="logs-toolbar"><span>{state.logs.length} событий · пароли и хеши скрыты</span><div><button class="btn" on:click={copyLogs}><Icon name="copy" size={16}/>Копировать</button><button class="btn" on:click={()=>void backend().ExportLog().catch(e=>error=String(e))}><Icon name="download" size={16}/>Сохранить</button></div></div><div class="card log-box" bind:this={logBox} role="textbox" aria-readonly="true" aria-multiline="true" tabindex="0" aria-label="Журнал событий">{#if page==='logs'}{#each state.logs as line}<div class="log-line" class:error-line={line.level==='error'}><time>{line.time}</time><span>{line.message}</span></div>{:else}<div class="empty"><Icon name="logs" size={35}/><h2>Журнал пока пуст</h2><p>Здесь появятся события после подключения.</p></div>{/each}{/if}</div></div>
 <footer><span><Icon name="shield" size={14}/>Настройки остаются на вашем компьютере</span><button class="text-button" on:click={hide}>Свернуть в трей<Icon name="arrow" size={13}/></button></footer>
 </main>
</div>
<UpdateDialog update={modal} current={version} {later} open={()=>void openUpdate()}/>
