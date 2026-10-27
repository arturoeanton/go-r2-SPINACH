import test from 'node:test';
import assert from 'node:assert/strict';
import {advanceUntilDone} from '../internal/app/web/run-controller.js';

test('pause finishes the in-flight action without scheduling another, then resumes', async () => {
  const run={index:0,auto:true,done:false};
  let release,calls=0;
  const pending=advanceUntilDone(run,async()=>{
    calls++;
    await new Promise(resolve=>{release=resolve});
    run.index++;
  },()=>true);
  assert.equal(calls,1);
  run.auto=false;
  release();
  await pending;
  assert.equal(calls,1);
  assert.equal(run.index,1);
  run.auto=true;
  await advanceUntilDone(run,async()=>{
    calls++;
    run.index++;
    if(run.index===3)run.done=true;
  },()=>true);
  assert.equal(calls,3);
});

test('tool errors can be corrected, while failed requests stop the loop', async()=>{
  const run={index:0,auto:true,done:false};
  await advanceUntilDone(run,async()=>{
    run.index++;
    run.error=true;
    if(run.index===2)run.fatal=true;
  },()=>true);
  assert.equal(run.index,2);
});
