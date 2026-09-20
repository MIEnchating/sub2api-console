// Host compatibility only. The current official SDK supplies all security
// token computation. Go owns networking, deadlines and the login state machine.
var window = globalThis, self = globalThis, top = globalThis;
var __config=JSON.parse(__configJSON);
var setTimeout=__setTimer,clearTimeout=__clearTimer;
var setInterval=function(fn,ms){function next(){fn();__setTimer(next,ms)}return __setTimer(next,ms)},clearInterval=__clearTimer;
var btoa=__btoa,atob=__atob;
var console={log:function(){},warn:function(){},error:function(){},debug:function(){}};
class URL {constructor(raw,base){Object.assign(this,__url(String(raw),base==null?'':String(base)))}toString(){return this.href}}
class URLSearchParams {constructor(raw){this.entriesArray=String(raw||'').replace(/^\?/,'').split('&').filter(Boolean).map(v=>{var i=v.indexOf('=');return [decodeURIComponent((i<0?v:v.slice(0,i)).replace(/\+/g,' ')),decodeURIComponent((i<0?'':v.slice(i+1)).replace(/\+/g,' '))]})}keys(){return this.entriesArray.map(v=>v[0])[Symbol.iterator]()}entries(){return this.entriesArray[Symbol.iterator]()}get(key){var v=this.entriesArray.find(v=>v[0]===key);return v?v[1]:null}[Symbol.iterator](){return this.entries()}}
var location=new URL(__config.pageURL);
var crypto={randomUUID:__uuid,getRandomValues:function(array){var bytes=__random(array.byteLength);new Uint8Array(array.buffer,array.byteOffset,array.byteLength).set(bytes);return array}};
var navigator=Object.create({userAgent:'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36',language:'en-US',languages:['en-US','en'],platform:'Win32',hardwareConcurrency:8,deviceMemory:8});
var screen={width:1920,height:1080,availWidth:1920,availHeight:1080,colorDepth:24,pixelDepth:24};
var performance={timeOrigin:Date.now(),now:function(){return Date.now()-this.timeOrigin},memory:{jsHeapSizeLimit:4294705152,totalJSHeapSize:0,usedJSHeapSize:0}};
class Event {constructor(type,options){this.type=type;Object.assign(this,options||{})}}
class MessageEvent extends Event {}
class EventTarget {constructor(){this.listeners={}}addEventListener(type,fn){(this.listeners[type]||(this.listeners[type]=[])).push(fn)}removeEventListener(type,fn){this.listeners[type]=(this.listeners[type]||[]).filter(v=>v!==fn)}dispatchEvent(event){for(var fn of (this.listeners[event.type]||[]).slice())fn.call(this,event);if(typeof this['on'+event.type]==='function')this['on'+event.type](event);return true}}
var __events=new EventTarget();
var addEventListener=__events.addEventListener.bind(__events),removeEventListener=__events.removeEventListener.bind(__events),dispatchEvent=__events.dispatchEvent.bind(__events);
var requestIdleCallback=function(fn){return setTimeout(()=>fn({didTimeout:false,timeRemaining:()=>1}),0)},cancelIdleCallback=clearTimeout;
class TextEncoder {encode(value){var raw=unescape(encodeURIComponent(String(value)));return Uint8Array.from(raw,c=>c.charCodeAt(0))}}
class TextDecoder {decode(value){return decodeURIComponent(escape(String.fromCharCode(...new Uint8Array(value.buffer||value))))}}
class Headers {constructor(values){this.values=Object.assign({},values||{})}set(k,v){this.values[k.toLowerCase()]=String(v)}get(k){return this.values[k.toLowerCase()]||null}entries(){return Object.entries(this.values)[Symbol.iterator]()}}
var fetch=async function(raw,options){options=options||{};var url=new URL(raw,location.href).href;var response=__request({URL:url,Method:options.method||'GET',Body:options.body||''});return {ok:response.status>=200&&response.status<300,status:response.status,text:async()=>response.body,json:async()=>JSON.parse(response.body)}};
class Element extends EventTarget {constructor(tag){super();this.localName=tag;this.tagName=tag.toUpperCase();this.style={};this.children=[];this.attributes={}}setAttribute(k,v){this.attributes[k]=String(v);this[k]=String(v)}getAttribute(k){return this.attributes[k]||null}appendChild(node){this.children.push(node);if(node.localName==='iframe')setTimeout(()=>node.dispatchEvent(new Event('load')),0);return node}removeChild(node){this.children=this.children.filter(v=>v!==node);return node}querySelector(){return null}querySelectorAll(){return []}}
var document=new EventTarget();
document.cookie='oai-did='+__config.deviceID;
document.currentScript={src:__config.sdkURL};document.scripts=[document.currentScript];document.documentElement=new Element('html');document.body=new Element('body');document.head=new Element('head');document.readyState='complete';document.referrer='';
document.createElement=function(tag){var node=new Element(tag);if(tag==='iframe'){
 // The SDK's frame only relays requirements requests and results. No page is
 // navigated: the Go boundary restricts this request to the official endpoint.
 node.contentWindow={postMessage:function(message){setTimeout(function(){try{
  var reply=__request({URL:'https://sentinel.openai.com/backend-api/sentinel/req',Method:'POST',Body:JSON.stringify({p:message.p,id:__config.deviceID,flow:message.flow})});
  if(reply.status!==200)throw Error('SDK request rejected');
  dispatchEvent(new MessageEvent('message',{source:node.contentWindow,origin:'https://sentinel.openai.com',data:{type:'response',requestId:message.requestId,result:{cachedChatReq:JSON.parse(reply.body),cachedProof:message.p}}}));
 }catch(error){dispatchEvent(new MessageEvent('message',{source:node.contentWindow,origin:'https://sentinel.openai.com',data:{type:'response',requestId:message.requestId,error:'SDK request failed'}}))}},0)}};
 }return node};
document.querySelector=()=>null;document.querySelectorAll=()=>[];document.getElementById=()=>null;document.getElementsByTagName=tag=>tag==='script'?document.scripts:[];
var localStorage={getItem:()=>null,setItem:()=>{},removeItem:()=>{}},sessionStorage=localStorage;
