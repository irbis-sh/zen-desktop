export namespace config {
	
	export enum RoutingMode {
	    BLOCKLIST = "blocklist",
	    ALLOWLIST = "allowlist",
	}
	export enum UpdatePolicyType {
	    AUTOMATIC = "automatic",
	    PROMPT = "prompt",
	    DISABLED = "disabled",
	}
	export class FilterList {
	    name: string;
	    type: string;
	    url: string;
	    enabled: boolean;
	    trusted: boolean;
	    locales: string[];
	
	    static createFrom(source: any = {}) {
	        return new FilterList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.url = source["url"];
	        this.enabled = source["enabled"];
	        this.trusted = source["trusted"];
	        this.locales = source["locales"];
	    }
	}
	export class LocalResourceMapping {
	    id: string;
	    library: string;
	    version: string;
	    patterns: string[];
	    file: string;
	    contentType: string;
	    versionRange?: string;
	    sri?: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalResourceMapping(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.library = source["library"];
	        this.version = source["version"];
	        this.patterns = source["patterns"];
	        this.file = source["file"];
	        this.contentType = source["contentType"];
	        this.versionRange = source["versionRange"];
	        this.sri = source["sri"];
	    }
	}
	export class LocalResourcesStats {
	    totalSinceInstall: number;
	    totalSinceReset: number;
	    filterHits: number;
	    byLibrary: Record<string, number>;
	    byCDN: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new LocalResourcesStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalSinceInstall = source["totalSinceInstall"];
	        this.totalSinceReset = source["totalSinceReset"];
	        this.filterHits = source["filterHits"];
	        this.byLibrary = source["byLibrary"];
	        this.byCDN = source["byCDN"];
	    }
	}
	export class LocalResources {
	    enabled: boolean;
	    blockMissing: boolean;
	    customDir: string;
	    enabledLibraries: string[];
	    customMappings: LocalResourceMapping[];
	    stats: LocalResourcesStats;
	
	    static createFrom(source: any = {}) {
	        return new LocalResources(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.blockMissing = source["blockMissing"];
	        this.customDir = source["customDir"];
	        this.enabledLibraries = source["enabledLibraries"];
	        this.customMappings = this.convertValues(source["customMappings"], LocalResourceMapping);
	        this.stats = this.convertValues(source["stats"], LocalResourcesStats);
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
	
	export class RoutingConfig {
	    mode: RoutingMode;
	    appPaths: string[];
	
	    static createFrom(source: any = {}) {
	        return new RoutingConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.appPaths = source["appPaths"];
	    }
	}

}

export namespace localcdn {
	
	export class LibraryInfo {
	    key: string;
	    name: string;
	    license: string;
	    version: string;
	    enabled: boolean;
	    resourceCount: number;
	
	    static createFrom(source: any = {}) {
	        return new LibraryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.license = source["license"];
	        this.version = source["version"];
	        this.enabled = source["enabled"];
	        this.resourceCount = source["resourceCount"];
	    }
	}

}

export namespace options {
	
	export class SecondInstanceData {
	    Args: string[];
	    WorkingDirectory: string;
	
	    static createFrom(source: any = {}) {
	        return new SecondInstanceData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Args = source["Args"];
	        this.WorkingDirectory = source["WorkingDirectory"];
	    }
	}

}

