export namespace config {
	
	export class ConfigFileInfo {
	    path: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.label = source["label"];
	    }
	}
	export class LogEntry {
	    // Go type: time
	    timestamp: any;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.level = source["level"];
	        this.message = source["message"];
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
	    pinned: boolean;
	    sourceFile?: string;
	    sourceFileLabel?: string;
	
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
	        this.pinned = source["pinned"];
	        this.sourceFile = source["sourceFile"];
	        this.sourceFileLabel = source["sourceFileLabel"];
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

export namespace main {
	
	export class SFTPOpenResult {
	    sessionId: string;
	    home: string;
	
	    static createFrom(source: any = {}) {
	        return new SFTPOpenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.home = source["home"];
	    }
	}
	export class SFTPUploadPick {
	    paths: string[];
	    conflicts: string[];
	
	    static createFrom(source: any = {}) {
	        return new SFTPUploadPick(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.paths = source["paths"];
	        this.conflicts = source["conflicts"];
	    }
	}

}

export namespace ssh {
	
	export class FileEntry {
	    name: string;
	    path: string;
	    size: number;
	    mode: string;
	    isDir: boolean;
	    isLink: boolean;
	    // Go type: time
	    modTime: any;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.isDir = source["isDir"];
	        this.isLink = source["isLink"];
	        this.modTime = this.convertValues(source["modTime"], null);
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

export namespace updater {
	
	export class UpdateInfo {
	    latestVersion: string;
	    releaseUrl: string;
	    assetUrl: string;
	    releaseNotes: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.latestVersion = source["latestVersion"];
	        this.releaseUrl = source["releaseUrl"];
	        this.assetUrl = source["assetUrl"];
	        this.releaseNotes = source["releaseNotes"];
	    }
	}

}

