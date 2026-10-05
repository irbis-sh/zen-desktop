export namespace config {
	
	export enum KeyStorageType {
	    DISK = "disk",
	    HARDWARE = "hardware",
	}
	export enum RoutingMode {
	    ALLOWLIST = "allowlist",
	    BLOCKLIST = "blocklist",
	}
	export enum UpdatePolicyType {
	    AUTOMATIC = "automatic",
	    DISABLED = "disabled",
	    PROMPT = "prompt",
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

