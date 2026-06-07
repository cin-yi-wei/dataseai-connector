export namespace main {
	
	export class GUIStatus {
	    service_status: string;
	    agent_status: string;
	    config_path: string;
	    server: string;
	    executor: string;
	    token_masked: string;
	    log_lines: string[];
	
	    static createFrom(source: any = {}) {
	        return new GUIStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.service_status = source["service_status"];
	        this.agent_status = source["agent_status"];
	        this.config_path = source["config_path"];
	        this.server = source["server"];
	        this.executor = source["executor"];
	        this.token_masked = source["token_masked"];
	        this.log_lines = source["log_lines"];
	    }
	}

}

