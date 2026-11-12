const $ = id => document.getElementById(id);
const els = {file:$('file'),level:$('level'),from:$('from'),to:$('to'),query:$('query'),regex:$('regex'),rows:$('rows'),status:$('status')};
let lastController=null;
function setStatus(s){els.status.textContent=s}
function esc(s){return String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
async function loadFiles(){
  try{
    const res=await fetch('/api/files'); if(!res.ok)throw new Error(await res.text());
    const files=await res.json(); els.file.innerHTML='';
    if(!files.length){els.file.innerHTML='<option value="">No supported files found</option>';return}
    files.forEach(f=>{const o=document.createElement('option');o.value=f;o.textContent=f;els.file.appendChild(o)});
    $('filename').textContent=files[0]; runSearch();
  }catch(e){setStatus('Could not load files');showEmpty('Unable to load files',e.message)}
}
function showEmpty(title,desc){els.rows.innerHTML=`<div class="empty"><div class="empty-icon">⌕</div><strong>${esc(title)}</strong><p>${esc(desc||'')}</p></div>`}
function levelClass(level){return 'lv-'+(level||'').toLowerCase()}
async function runSearch(){
  if(!els.file.value){showEmpty('Choose a log file','No log file is selected.');return}
  if(lastController)lastController.abort(); lastController=new AbortController();
  const p=new URLSearchParams({file:els.file.value,q:els.query.value,level:els.level.value,regex:String(els.regex.checked),limit:'500'});
  if(els.from.value)p.set('from',els.from.value.replace('T',' ')+':00');
  if(els.to.value)p.set('to',els.to.value.replace('T',' ')+':59');
  setStatus('Scanning…');$('filename').textContent=els.file.value;
  try{
    const res=await fetch('/api/search?'+p,{signal:lastController.signal});
    const data=await res.json(); if(!res.ok)throw new Error(data.error||'Search failed');
    $('total').textContent=data.total.toLocaleString();$('matches').textContent=data.matched.toLocaleString();$('displayed').textContent=data.entries.length.toLocaleString();
    $('result-note').textContent=`Showing ${data.entries.length.toLocaleString()} of ${data.matched.toLocaleString()} matching entries`;
    if(!data.entries.length){showEmpty('No matching entries','Try broadening your search or clearing some filters.')}
    else{
      els.rows.innerHTML=data.entries.map(e=>`<div class="entry"><span class="line-no">${e.line}</span><span class="timestamp">${esc(e.time||'—')}</span><span>${e.level?`<b class="level ${levelClass(e.level)}">${esc(e.level)}</b>`:'<span class="line-no">—</span>'}</span><span class="message">${esc(e.text)}</span></div>`).join('');
      if(data.truncated)els.rows.insertAdjacentHTML('beforeend','<div class="truncated">Result limit reached. Refine your filters to see a narrower result set.</div>');
    }
    setStatus(`Scanned ${data.total.toLocaleString()} lines`);
  }catch(e){if(e.name!=='AbortError'){setStatus('Search error');showEmpty('Search failed',e.message)}}
}
$('search').addEventListener('click',runSearch);$('refresh').addEventListener('click',()=>{loadFiles()});
$('clear').addEventListener('click',()=>{els.query.value='';els.level.value='ALL';els.from.value='';els.to.value='';els.regex.checked=false;runSearch()});
els.file.addEventListener('change',runSearch);els.level.addEventListener('change',runSearch);
els.query.addEventListener('keydown',e=>{if(e.key==='Enter')runSearch()});
[els.from,els.to].forEach(el=>el.addEventListener('change',runSearch));
loadFiles();
