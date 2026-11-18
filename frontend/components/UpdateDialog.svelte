<script lang="ts">
 import {tick} from 'svelte';
 import Icon from './Icon.svelte';
 import type {Update} from '../types';
 export let update:Update|null=null;
 export let current='';
 export let later:()=>void;
 export let open:()=>void;
 let dialog:HTMLDialogElement;
 $: if(update) void tick().then(()=>{if(dialog&&!dialog.open)dialog.showModal()});
 $: if(!update&&dialog?.open)dialog.close();
</script>
<dialog bind:this={dialog} class="update-dialog" on:close={later} aria-labelledby="update-title" aria-describedby="update-description">
 {#if update}
 <div class="update-icon"><Icon name="download" size={27}/></div><span class="eyebrow">Доступно обновление</span>
 <h2 id="update-title">CSQTT v{update.version}</h2>
 <p id="update-description">Вышла новая версия приложения. Посмотрите изменения и скачайте архив на странице релиза.</p>
 <div class="version-change"><span>Установлена v{current}</span><Icon name="arrow" size={15}/><strong>v{update.version}</strong></div>
 <div class="dialog-actions"><button class="btn ghost" on:click={()=>{later();dialog.close()}}>Позже</button><button class="btn primary" on:click={open}>Обновить<Icon name="arrow" size={16}/></button></div>
 <small>Откроется страница GitHub. Скачивание начнёте сами.</small>
 {/if}
</dialog>
