import { html, Component, render } from 'https://unpkg.com/htm/preact/standalone.module.js';
const socket = new WebSocket(`${location.origin.replace(/^http/, 'ws')}/socket`);

let toggle = (id, childId) => {
    socket.send(JSON.stringify({ type: "TOGGLE", params: { childId: childId, id: id } }));
};

function Child(props) {
    let child = props.child;
    let id = props.id;
    let btnState = "";
    if (child.state === 0) {
        btnState = "button-outline";
    }
    return html`<li><button class="button ${btnState}" onClick=${() => { toggle(id, child.id); }}>Toggle State</button> - ${child.alias} - ${child.state === 1 ? "on" : "off"}</li>`
}

function Plug(props) {
    let children = props.plg.system.get_sysinfo.children.map((child) => {
        return html`<${Child} id=${props.plg.system.get_sysinfo.deviceId} child=${child}/>`
    })
    return html`<p>${props.plg.system.get_sysinfo.alias}</p>
    <ul>
    ${children}
    </ul>
    `
}

class App extends Component {
    constructor(props) {
        super(props)
        this.state = {}
        this.handleMessage = this.handleMessage.bind(this);
    }

    handleMessage(event) {
        let plgMap = JSON.parse(event.data);
        this.setState({ plgMap: plgMap })
    }

    componentDidMount() {
        socket.addEventListener('message', this.handleMessage);
    }

    render() {
        if (this.state.plgMap) {
            let plugs = [];
            for (var id in this.state.plgMap) {
                let plg = this.state.plgMap[id];
                plugs.push(html`<${Plug} plg="${plg}" />`);
            }

            return html`<div class="container">${plugs}</div>`


        }
        return html`<h1>Loading!</h1>`;
    }
}

render(html`<${App} />`, document.body);