import {writeFileSync} from 'node:fs'
export default function teardown(){
 if(!process.env.AGENT_BROWSER_FIXTURE_FILE)throw Error('Owned agent fixture is required')
 writeFileSync(process.env.AGENT_BROWSER_FIXTURE_FILE+'.stop','stop')
}
