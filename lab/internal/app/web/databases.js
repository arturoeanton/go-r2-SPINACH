// The relational explorer is a separate UI module; source databases remain read-only.
export function databaseForm({form,api,loadGraphs,state,toast,esc,field}){
 form('Una base de datos, otro universo',`<div class="help-copy"><p>Cargá un archivo SQLite y descubrí automáticamente sus tablas, columnas y relaciones declaradas. La app consulta una copia local de solo lectura y dibuja una vista parcial.</p></div>${field('Nombre visible','title','','placeholder="Mi base de datos"')}<label class="form-field">Archivo SQLite<input type="file" name="file" accept=".sqlite,.sqlite3,.db" required><small>Hasta 16 MB en el laboratorio. Para bases mayores usá SPARQLite por separado. Si la base usa WAL, generá primero una copia consistente con su herramienta de respaldo.</small></label>`,async f=>{let file=f.get('file');if(file.size>16*1024*1024)throw Error('El archivo supera los 16 MB; usá el servidor independiente');let bytes=new Uint8Array(await file.arrayBuffer()),binary='';for(let i=0;i<bytes.length;i+=32768)binary+=String.fromCharCode(...bytes.subarray(i,i+32768));let g=await api('/databases',{title:f.get('title').trim()||file.name,content:btoa(binary)});state.dirty=false;await loadGraphs(g.id);toast('Base inspeccionada. Las consultas acceden a todos sus datos.')},'Explorar base')
}
export async function schemaModal({api,state,modal,esc}){
 let schema=await api('/databases/'+state.g.sqliteSource+'/schema');modal('El mapa de tu SQLite',`<div class="help-copy"><p>Este vocabulario se genera con la estructura declarada de la base. Los nombres son deterministas y las relaciones no se adivinan.</p>${schema.tables.map(t=>`<h3>${esc(t.name)} <span class="tag">${esc(t.kind)}</span></h3><p>Identidad: ${esc(t.identity)} · ${t.indexes.length} índices</p><div class="results-wrap"><table><thead><tr><th>Columna</th><th>Tipo</th><th>Predicado RDF</th></tr></thead><tbody>${t.columns.map(c=>`<tr><td>${esc(c.name)} ${c.primaryKey?'🔑':''}</td><td>${esc(c.type||'dinámico')}</td><td><code>${esc(c.predicate)}</code></td></tr>`).join('')}</tbody></table></div>${t.foreignKeys.map(f=>`<p>↗ ${esc(f.from.join(', '))} → ${esc(f.table)} · <code>${esc(f.predicate)}</code></p>`).join('')}`).join('')}<h3>Observaciones del mapeo</h3>${schema.warnings.map(w=>`<p>${esc(w)}</p>`).join('')}</div>`)
}

// Capture the selected graph so confirmation always applies to the named upload.
export function deleteDatabaseForm({form,api,loadGraphs,state,toast,esc,stopRun}){
 const graph=state.g;
 if(!graph?.sqliteSource)return;
 const shared=state.graphs.some(g=>g.id!==graph.id&&g.sqliteSource===graph.sqliteSource);
 form('Eliminar base SQLite',`<div class="help-copy"><p><strong data-no-i18n>${esc(graph.title)}</strong></p><p>Se eliminarán este universo y sus subgrafos guardados. El archivo original de tu computadora no se modifica.</p><p>${shared?'La copia SQLite seguirá disponible para los otros universos que la usan.':'También se eliminará la copia SQLite guardada por la app.'}</p><p>Esta acción no se puede deshacer. Podés volver a importar el archivo cuando quieras.</p></div>`,async()=>{
   stopRun(false);
   await api('/graphs/'+graph.id,{},'DELETE');
   state.dirty=false;
   await loadGraphs();
   toast('Base SQLite eliminada del laboratorio.');
 },'Eliminar SQLite');
 const submit=document.querySelector('#edit-form button[type="submit"]');
 submit.classList.remove('primary');submit.classList.add('danger');
}
