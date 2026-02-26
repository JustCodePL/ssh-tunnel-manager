export namespace config {
	
	export class PortForward {
	    localPort: number;
	    remoteHost: string;
	    remotePort: number;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new PortForward(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localPort = source["localPort"];
	        this.remoteHost = source["remoteHost"];
	        this.remotePort = source["remotePort"];
	        this.description = source["description"];
	    }
	}
	export class TunnelConfig {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    keyPath: string;
	    portForwards: PortForward[];
	    proxyCommand?: string;
	    proxyJump?: string;
	    color?: string;
	    group: string;
	    autoConnect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TunnelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.keyPath = source["keyPath"];
	        this.portForwards = this.convertValues(source["portForwards"], PortForward);
	        this.proxyCommand = source["proxyCommand"];
	        this.proxyJump = source["proxyJump"];
	        this.color = source["color"];
	        this.group = source["group"];
	        this.autoConnect = source["autoConnect"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

