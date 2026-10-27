import {t} from './i18n.js';
import {formatDuration} from './run-metrics.js';

const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const json=value=>esc(JSON.stringify(value,null,2));
const provider=name=>({openai:'OpenAI',jev:'Jev'})[name]||name;
export const actionTitle=type=>({search:'Buscar entidades',explore:'Explorar conexiones',query:'Ejecutar consulta',answer:'Verificar respuesta'})[type]||'Acción';

function inputs(trace,graph){
  let generation=trace.generation,input=generation?.input||trace.input;
  if(!input)return '';
  let focus=graph.subgraphs.find(s=>s.id===input.activeSubgraph)?.name;
  return `<section class="trace-section"><h4>${t('Información utilizada')}</h4>${input.question?`<p>${esc(input.question)}</p>`:''}${generation?`<div class="trace-facts"><span>${t('Contexto del grafo')}: ${input.entityDirectory?.length||0} ${t('entidades')} · ${input.initialSubgraph?.length||0} ${t('conexiones')}</span><span>${t('Región')}: ${esc(focus?t(focus):t('Vista del grafo completo'))}</span><span>${t('Observaciones anteriores')}: ${input.observations?.length||0}</span></div>`:''}<details class="trace-inspect"><summary>${t(generation?'Ver contexto enviado al generador':'Ver datos de entrada')}</summary><pre class="trace-code">${json(input)}</pre></details>${generation?.instructions?`<details class="trace-inspect"><summary>${t('Ver instrucciones del sistema')}</summary><pre class="trace-code">${esc(generation.instructions)}</pre></details>`:''}</section>`;
}
function navigation(trace,graph){
  let n=trace.navigation;if(!n)return '';
  let region=graph.subgraphs.find(s=>s.id===n.choice)?.name||n.choice||t('Todo el universo');
  return `<section class="trace-section"><h4>${t('Navegación')} <span>${n.source==='regla'?t('Regla, sin inferencia'):esc(n.model)+' · '+provider(n.source)}${trace.timing?' · '+formatDuration(trace.timing.navigationMs):''}</span></h4><p>${trace.error&&!n.choice?t('No se seleccionó un subgrafo.'):`${t('Subgrafo elegido')}: <strong>${esc(t(region))}</strong>`}</p>${n.candidates?.length?`<details class="trace-inspect"><summary>${t('Ver opciones y puntuaciones')}</summary>${n.candidates.map(c=>`<div class="trace-score"><span>${esc(t(c.name))}</span><strong>${(c.probability*100).toFixed(1)}%</strong></div>`).join('')}<p class="option-hint">${t('Estas puntuaciones priorizan la exploración; no expresan certeza sobre la respuesta.')}</p></details>`:''}${n.input?`<details class="trace-inspect"><summary>${t('Ver contexto de navegación')}</summary><pre class="trace-code">${json(n.input)}</pre></details>`:''}</section>`;
}
function action(trace){
  let a=trace.action;if(!a)return '';
  let value=a.query||a.text||a.nodeId||a.answer;
  return `<section class="trace-section"><h4>${t('Acción ejecutada')}${trace.generation?` <span>${esc(trace.generation.model)} · ${esc(provider(trace.generation.provider))}${trace.timing?' · '+formatDuration(trace.timing.generationMs):''}</span>`:''}</h4>${a.objective?`<p>${esc(a.objective)}</p>`:''}${value?`<pre class="trace-code">${esc(value)}</pre>`:''}</section>`;
}
function output(trace,index){
  if(trace.navigation&&!trace.action&&!trace.error)return '';
  if(trace.error)return `<section class="trace-section trace-error"><h4>${t('Error')}</h4><p>${esc(trace.error)}</p></section>`;
  let result=trace.observation,query=trace.action?.type==='query';
  let summary=trace.outcome;
  if(query&&result)summary=result.boolean!==undefined?`ASK → ${result.boolean}`:`${result.rows?.length||0} ${t('filas obtenidas')}`;
  else if(Array.isArray(result))summary=`${result.length} ${t('elementos encontrados')}`;
  let preview=query&&result?.rows?.length?`<ul class="trace-results">${result.rows.slice(0,3).map(row=>`<li>${result.variables.map(v=>esc(row[v]?.label||row[v]?.value||'—')).join(' · ')}</li>`).join('')}</ul>`:'';
  return `<section class="trace-section"><h4>${t('Resultado observable')}${trace.timing?` <span>${t('Herramientas')}: ${formatDuration(trace.timing.toolMs)}</span>`:''}</h4>${summary?`<p>${esc(t(summary))}</p>`:''}${preview}${query&&result?`<button class="text-button trace-evidence" data-trace="${index}">${t('Ver tabla de evidencia')} →</button>`:''}${trace.action?.type==='answer'?`<p>${t('Contrastá la síntesis con las consultas anteriores.')}</p>`:''}</section>`;
}
export function traceSteps(run,graph,stages){
  let traces=(run?.traces||[]).map(trace=>run.measureTime?trace:{...trace,timing:null});
  let html=traces.map((trace,i)=>`<li class="journey-step ${trace.error?'trace-failed':'done'}"><details class="trace-card" ${i===traces.length-1?'open':''}><summary><span class="step-number">${String(i+1).padStart(2,'0')}</span><span class="trace-heading"><strong>${t(trace.title)}</strong><span>${esc(t(trace.actor||'Regla, sin inferencia'))}</span></span><span class="trace-time">${run.measureTime&&trace.durationMs!==undefined?formatDuration(trace.durationMs):t('Sin medición')}</span></summary><div class="trace-body">${trace.description?`<p class="trace-description">${esc(t(trace.description))}</p>`:''}${navigation(trace,graph)}${inputs(trace,graph)}${action(trace)}${output(trace,i)}${trace.timing?.replayMs?`<p class="option-hint">${t('Revalidar evidencia')}: ${formatDuration(trace.timing.replayMs)}</p>`:''}</div></details></li>`).join('');
  if(!run){
    let offset=run?run.index+1:0;
    html+=stages.slice(offset).map(([title,desc],i)=>`<li class="journey-step trace-pending"><span class="step-number">${String(offset+i+1).padStart(2,'0')}</span><div><strong>${t(title)}</strong><p>${t(desc)} · ${t('Pendiente')}</p></div></li>`).join('');
  }
  return html;
}
