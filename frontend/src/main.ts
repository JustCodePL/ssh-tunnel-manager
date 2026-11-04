import './app.css'
import '@xterm/xterm/css/xterm.css'
import App from './App.svelte'
import ToolApp from './ToolApp.svelte'

const target = document.getElementById('app') as HTMLElement
const windowName = new URLSearchParams(location.search).get('window')

const app = windowName
  ? new ToolApp({ target, props: { windowName } })
  : new App({ target })

export default app
