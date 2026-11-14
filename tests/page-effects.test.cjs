const assert = require('node:assert/strict');
const test = require('node:test');
const { edgeFadeHeights, swipeDecision, springStep } = require('../web/static/page-effects.js');

test('edge fade follows the furthest measured control plus 50 px', () => {
  assert.deepEqual(edgeFadeHeights({top: 0, bottom: 844}, [{bottom:84}, {bottom:110}], [{top:760}, {top:720}]), {top:160,bottom:174});
  assert.deepEqual(edgeFadeHeights({top:20,bottom:820}), {top:50,bottom:50});
});
test('short flicks commit; upward reversal and short slow swipes cancel', () => {
  assert.equal(swipeDecision(50,600,390),true);
  assert.equal(swipeDecision(180,0,390),true);
  assert.equal(swipeDecision(180,-300,390),false);
  assert.equal(swipeDecision(50,0,390),false);
  assert.equal(swipeDecision(20,900,390),false);
});
test('critically damped cancellation settles without bounce; frames carry velocity', () => {
  let x=180,velocity=0, previous=x;
  for(let i=0;i<90;i++) {
    ({x,velocity}=springStep(x,velocity,0,1/60));
    assert.ok(x>=0 && x<=previous); previous=x;
  }
  assert.ok(x<.01);
  assert.ok(springStep(80,700,390,1/60).x>springStep(80,0,390,1/60).x);
});
