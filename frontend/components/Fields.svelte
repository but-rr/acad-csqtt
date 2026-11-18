<script lang="ts">
 import type { Profile } from '../types';
 export let profile:Profile;
 export let fields:{key:keyof Profile;label:string;secret?:boolean;hint?:string;options?:string[]}[]=[];
 export let changed:()=>void=()=>{};
 export let disabled=false;
 let revealed:Partial<Record<keyof Profile,boolean>>={};
</script>
{#each fields as field}
 <label class="field"><span>{field.label}</span><div class="input-row">
 {#if field.options}<select {disabled} bind:value={profile[field.key]} on:change={changed}>{#each field.options as option}<option>{option}</option>{/each}</select>
 {:else}<input {disabled} type={field.secret&&!revealed[field.key]?'password':'text'} bind:value={profile[field.key]} on:input={changed} autocomplete="off" spellcheck="false"/>{/if}
 {#if field.secret}<button type="button" class="reveal" aria-label={revealed[field.key]?'Скрыть '+field.label:'Показать '+field.label} on:click={()=>revealed[field.key]=!revealed[field.key]}>{revealed[field.key]?'Скрыть':'Показать'}</button>{/if}
 </div>{#if field.hint}<small>{field.hint}</small>{/if}</label>
{/each}
