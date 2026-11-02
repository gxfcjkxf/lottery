import {defineComponent,h} from 'vue'

// Delegate table slots in host-renderer business tests. Shared table layout is
// exercised by the real component in the browser review.
export const ManagementTableTestComponent=defineComponent({
 props:['entries','columns','empty','emptyLabel'],
 setup(props,{slots}){return()=>h('table',[
  h('thead',[h('tr',(props.columns??[]).map((c:any)=>h('th',c.label)))]),
  h('tbody',[(props.entries??[]).map((entry:any)=>h('tr',(props.columns??[]).map((c:any)=>h('td',slots[c.key]?.({entry}))))),props.empty?h('tr',[h('td',props.emptyLabel)]):null]),
 ])},
})
