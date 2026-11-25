<script lang="ts">
 import type {Profile} from '../types';
 import Fields from './Fields.svelte';
 export let profile:Profile;
 export let disabled=false;
 export let changed:()=>void=()=>{};
 $: servers=profile.dns_mode==='google'?'8.8.8.8 · 8.8.4.4':profile.dns_mode==='cloudflare'?'1.1.1.1 · 1.0.0.1':'';
</script>

<section class="card settings-card" aria-labelledby="dns-title">
 <h2 id="dns-title">DNS</h2>
 <p>DNS для локальных прокси и адаптера TUN. Применяется при подключении.</p>
 <label class="field"><span>Режим DNS</span>
  <select bind:value={profile.dns_mode} {disabled} on:change={changed}>
   <option value="server">От сервера CSQTT</option>
   <option value="google">Google · 8.8.8.8</option>
   <option value="cloudflare">Cloudflare · 1.1.1.1</option>
   <option value="custom">Свои DNS</option>
  </select>
 </label>
 {#if profile.dns_mode==='custom'}
  <Fields bind:profile {disabled} {changed} fields={[
   {key:'dns_primary',label:'Основной DNS',hint:'IPv4-адрес, например 8.8.8.8'},
   {key:'dns_secondary',label:'Резервный DNS',hint:'Необязательно, например 8.8.4.4'}
  ]}/>
 {:else if servers}<p><code>{servers}</code></p>
 {:else}<p>Используются DNS-адреса из конфигурации вашего сервера.</p>{/if}
 {#if disabled}<p>Отключитесь, чтобы изменить DNS.</p>{/if}
</section>
