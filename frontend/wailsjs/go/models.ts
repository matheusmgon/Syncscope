export namespace argocd {
	
	export class ActionDef {
	    name: string;
	    displayName?: string;
	    disabled: boolean;
	    iconClass?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.disabled = source["disabled"];
	        this.iconClass = source["iconClass"];
	    }
	}
	export class AppSource {
	    repoURL: string;
	    path?: string;
	    targetRevision?: string;
	    chart?: string;
	    ref?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoURL = source["repoURL"];
	        this.path = source["path"];
	        this.targetRevision = source["targetRevision"];
	        this.chart = source["chart"];
	        this.ref = source["ref"];
	    }
	}
	export class SyncWindow {
	    kind: string;
	    schedule: string;
	    duration: string;
	    applications?: string[];
	    namespaces?: string[];
	    clusters?: string[];
	    manualSync?: boolean;
	    timeZone?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncWindow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.schedule = source["schedule"];
	        this.duration = source["duration"];
	        this.applications = source["applications"];
	        this.namespaces = source["namespaces"];
	        this.clusters = source["clusters"];
	        this.manualSync = source["manualSync"];
	        this.timeZone = source["timeZone"];
	    }
	}
	export class AppSyncWindows {
	    assignedWindows: SyncWindow[];
	    activeWindows: SyncWindow[];
	    canSync: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppSyncWindows(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assignedWindows = this.convertValues(source["assignedWindows"], SyncWindow);
	        this.activeWindows = this.convertValues(source["activeWindows"], SyncWindow);
	        this.canSync = source["canSync"];
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
	export class Condition {
	    type: string;
	    message: string;
	    status?: string;
	    reason?: string;
	    lastTransitionTime?: string;
	
	    static createFrom(source: any = {}) {
	        return new Condition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.message = source["message"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.lastTransitionTime = source["lastTransitionTime"];
	    }
	}
	export class ResourceResult {
	    group: string;
	    version: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    status?: string;
	    message?: string;
	    hookPhase?: string;
	    hookType?: string;
	    syncPhase?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group = source["group"];
	        this.version = source["version"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.hookPhase = source["hookPhase"];
	        this.hookType = source["hookType"];
	        this.syncPhase = source["syncPhase"];
	    }
	}
	export class SyncResult {
	    revision?: string;
	    resources?: ResourceResult[];
	
	    static createFrom(source: any = {}) {
	        return new SyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.resources = this.convertValues(source["resources"], ResourceResult);
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
	export class OperationState {
	    phase: string;
	    message?: string;
	    startedAt?: string;
	    finishedAt?: string;
	    retryCount?: number;
	    syncResult?: SyncResult;
	
	    static createFrom(source: any = {}) {
	        return new OperationState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.message = source["message"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.retryCount = source["retryCount"];
	        this.syncResult = this.convertValues(source["syncResult"], SyncResult);
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
	export class RepoInput {
	    repo: string;
	    type: string;
	    name?: string;
	    project?: string;
	    username?: string;
	    password?: string;
	    sshPrivateKey?: string;
	    insecure?: boolean;
	    enableOCI?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RepoInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo = source["repo"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.project = source["project"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.sshPrivateKey = source["sshPrivateKey"];
	        this.insecure = source["insecure"];
	        this.enableOCI = source["enableOCI"];
	    }
	}
	export class ResourceAction {
	    Group: string;
	    Version: string;
	    Kind: string;
	    Namespace: string;
	    Name: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Group = source["Group"];
	        this.Version = source["Version"];
	        this.Kind = source["Kind"];
	        this.Namespace = source["Namespace"];
	        this.Name = source["Name"];
	    }
	}
	
	export class RevisionHistory {
	    id: number;
	    revision?: string;
	    revisions?: string[];
	    deployedAt: string;
	    deployStartedAt?: string;
	    source?: AppSource;
	
	    static createFrom(source: any = {}) {
	        return new RevisionHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.revision = source["revision"];
	        this.revisions = source["revisions"];
	        this.deployedAt = source["deployedAt"];
	        this.deployStartedAt = source["deployStartedAt"];
	        this.source = this.convertValues(source["source"], AppSource);
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
	export class SyncResource {
	    group: string;
	    kind: string;
	    name: string;
	    namespace: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncResource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group = source["group"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	    }
	}
	export class SyncOptions {
	    prune: boolean;
	    dryRun: boolean;
	    force: boolean;
	    applyOutOfSyncOnly: boolean;
	    resources?: SyncResource[];
	    revision?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prune = source["prune"];
	        this.dryRun = source["dryRun"];
	        this.force = source["force"];
	        this.applyOutOfSyncOnly = source["applyOutOfSyncOnly"];
	        this.resources = this.convertValues(source["resources"], SyncResource);
	        this.revision = source["revision"];
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

export namespace config {
	
	export class Context {
	    id: string;
	    name: string;
	    server: string;
	    authType: string;
	    insecure: boolean;
	    caFile?: string;
	    clientCertFile?: string;
	    clientKeyFile?: string;
	    headers?: Record<string, string>;
	    ssoPort?: number;
	    ssoNoOffline?: boolean;
	    username?: string;
	    color?: string;
	    disabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Context(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.server = source["server"];
	        this.authType = source["authType"];
	        this.insecure = source["insecure"];
	        this.caFile = source["caFile"];
	        this.clientCertFile = source["clientCertFile"];
	        this.clientKeyFile = source["clientKeyFile"];
	        this.headers = source["headers"];
	        this.ssoPort = source["ssoPort"];
	        this.ssoNoOffline = source["ssoNoOffline"];
	        this.username = source["username"];
	        this.color = source["color"];
	        this.disabled = source["disabled"];
	    }
	}
	export class Prefs {
	    theme?: string;
	    kubeContexts?: string[];
	    noUpdateCheck?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Prefs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.kubeContexts = source["kubeContexts"];
	        this.noUpdateCheck = source["noUpdateCheck"];
	    }
	}

}

export namespace kube {
	
	export class Event {
	    type: string;
	    reason: string;
	    message: string;
	    count: number;
	    first: string;
	    last: string;
	    object: string;
	    component: string;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.reason = source["reason"];
	        this.message = source["message"];
	        this.count = source["count"];
	        this.first = source["first"];
	        this.last = source["last"];
	        this.object = source["object"];
	        this.component = source["component"];
	    }
	}
	export class PodInfo {
	    name: string;
	    namespace: string;
	    phase: string;
	    reason?: string;
	    node?: string;
	    containers: string[];
	    init?: string[];
	    ready: string;
	    restarts: number;
	    created: string;
	    annotations?: Record<string, string>;
	    labels?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new PodInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.phase = source["phase"];
	        this.reason = source["reason"];
	        this.node = source["node"];
	        this.containers = source["containers"];
	        this.init = source["init"];
	        this.ready = source["ready"];
	        this.restarts = source["restarts"];
	        this.created = source["created"];
	        this.annotations = source["annotations"];
	        this.labels = source["labels"];
	    }
	}

}

export namespace kubestore {
	
	export class KindState {
	    state: string;
	    message?: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new KindState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.message = source["message"];
	        this.count = source["count"];
	    }
	}
	export class ContextStatus {
	    name: string;
	    server: string;
	    version?: string;
	    state: string;
	    message?: string;
	    kinds: Record<string, KindState>;
	
	    static createFrom(source: any = {}) {
	        return new ContextStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.server = source["server"];
	        this.version = source["version"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.kinds = this.convertValues(source["kinds"], KindState, true);
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
	export class ContextView {
	    name: string;
	    cluster: string;
	    server: string;
	    namespace: string;
	    user: string;
	    current: boolean;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContextView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cluster = source["cluster"];
	        this.server = source["server"];
	        this.namespace = source["namespace"];
	        this.user = source["user"];
	        this.current = source["current"];
	        this.enabled = source["enabled"];
	    }
	}
	
	export class LogRequest {
	    ctx: string;
	    namespace: string;
	    pod: string;
	    container: string;
	    tailLines: number;
	    follow: boolean;
	    previous: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LogRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ctx = source["ctx"];
	        this.namespace = source["namespace"];
	        this.pod = source["pod"];
	        this.container = source["container"];
	        this.tailLines = source["tailLines"];
	        this.follow = source["follow"];
	        this.previous = source["previous"];
	    }
	}
	export class Problem {
	    severity: string;
	    source: string;
	    resource?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.source = source["source"];
	        this.resource = source["resource"];
	        this.message = source["message"];
	    }
	}
	export class Obj {
	    key: string;
	    ctx: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    phase: string;
	    message?: string;
	    severity: number;
	    problems?: Problem[];
	    labels?: Record<string, string>;
	    created: string;
	    fields: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new Obj(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.ctx = source["ctx"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.phase = source["phase"];
	        this.message = source["message"];
	        this.severity = source["severity"];
	        this.problems = this.convertValues(source["problems"], Problem);
	        this.labels = source["labels"];
	        this.created = source["created"];
	        this.fields = source["fields"];
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

export namespace store {
	
	export class ActionResult {
	    key: string;
	    ctx: string;
	    name: string;
	    ok: boolean;
	    error?: string;
	    info?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.ctx = source["ctx"];
	        this.name = source["name"];
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.info = source["info"];
	    }
	}
	export class ActionReport {
	    id: string;
	    action: string;
	    total: number;
	    failed: number;
	    results: ActionResult[];
	    started: string;
	    elapsed: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.action = source["action"];
	        this.total = source["total"];
	        this.failed = source["failed"];
	        this.results = this.convertValues(source["results"], ActionResult);
	        this.started = source["started"];
	        this.elapsed = source["elapsed"];
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
	
	export class TreeNode {
	    id: string;
	    uid?: string;
	    group: string;
	    version: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    parents: string[];
	    health: string;
	    healthMsg?: string;
	    sync?: string;
	    managed: boolean;
	    hook: boolean;
	    prune: boolean;
	    restartable: boolean;
	    hasLogs: boolean;
	    info?: Record<string, string>;
	    images?: string[];
	    createdAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new TreeNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.uid = source["uid"];
	        this.group = source["group"];
	        this.version = source["version"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.parents = source["parents"];
	        this.health = source["health"];
	        this.healthMsg = source["healthMsg"];
	        this.sync = source["sync"];
	        this.managed = source["managed"];
	        this.hook = source["hook"];
	        this.prune = source["prune"];
	        this.restartable = source["restartable"];
	        this.hasLogs = source["hasLogs"];
	        this.info = source["info"];
	        this.images = source["images"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ResourceRow {
	    group: string;
	    version: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    sync: string;
	    health: string;
	    message?: string;
	    restartable: boolean;
	    hook: boolean;
	    prune: boolean;
	    parent?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group = source["group"];
	        this.version = source["version"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.sync = source["sync"];
	        this.health = source["health"];
	        this.message = source["message"];
	        this.restartable = source["restartable"];
	        this.hook = source["hook"];
	        this.prune = source["prune"];
	        this.parent = source["parent"];
	    }
	}
	export class Problem {
	    severity: string;
	    source: string;
	    resource?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.source = source["source"];
	        this.resource = source["resource"];
	        this.message = source["message"];
	    }
	}
	export class AppSummary {
	    key: string;
	    ctx: string;
	    name: string;
	    appNamespace: string;
	    project: string;
	    appSet: string;
	    cluster: string;
	    clusterServer: string;
	    destNamespace: string;
	    repo: string;
	    path: string;
	    targetRev: string;
	    syncRev: string;
	    sync: string;
	    health: string;
	    healthMsg?: string;
	    opPhase?: string;
	    opMessage?: string;
	    opFinishedAt?: string;
	    opStartedAt?: string;
	    autoSync: boolean;
	    deleting: boolean;
	    labels?: Record<string, string>;
	    problems?: Problem[];
	    severity: number;
	    workloads: number;
	    reconciledAt?: string;
	    createdAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.ctx = source["ctx"];
	        this.name = source["name"];
	        this.appNamespace = source["appNamespace"];
	        this.project = source["project"];
	        this.appSet = source["appSet"];
	        this.cluster = source["cluster"];
	        this.clusterServer = source["clusterServer"];
	        this.destNamespace = source["destNamespace"];
	        this.repo = source["repo"];
	        this.path = source["path"];
	        this.targetRev = source["targetRev"];
	        this.syncRev = source["syncRev"];
	        this.sync = source["sync"];
	        this.health = source["health"];
	        this.healthMsg = source["healthMsg"];
	        this.opPhase = source["opPhase"];
	        this.opMessage = source["opMessage"];
	        this.opFinishedAt = source["opFinishedAt"];
	        this.opStartedAt = source["opStartedAt"];
	        this.autoSync = source["autoSync"];
	        this.deleting = source["deleting"];
	        this.labels = source["labels"];
	        this.problems = this.convertValues(source["problems"], Problem);
	        this.severity = source["severity"];
	        this.workloads = source["workloads"];
	        this.reconciledAt = source["reconciledAt"];
	        this.createdAt = source["createdAt"];
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
	export class AppDetail {
	    summary: AppSummary;
	    sources: argocd.AppSource[];
	    conditions: argocd.Condition[];
	    operation?: argocd.OperationState;
	    resources: ResourceRow[];
	    pods: ResourceRow[];
	    history: argocd.RevisionHistory[];
	    webURL: string;
	    prune: boolean;
	    selfHeal: boolean;
	    treeError?: string;
	    tree: TreeNode[];
	
	    static createFrom(source: any = {}) {
	        return new AppDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = this.convertValues(source["summary"], AppSummary);
	        this.sources = this.convertValues(source["sources"], argocd.AppSource);
	        this.conditions = this.convertValues(source["conditions"], argocd.Condition);
	        this.operation = this.convertValues(source["operation"], argocd.OperationState);
	        this.resources = this.convertValues(source["resources"], ResourceRow);
	        this.pods = this.convertValues(source["pods"], ResourceRow);
	        this.history = this.convertValues(source["history"], argocd.RevisionHistory);
	        this.webURL = source["webURL"];
	        this.prune = source["prune"];
	        this.selfHeal = source["selfHeal"];
	        this.treeError = source["treeError"];
	        this.tree = this.convertValues(source["tree"], TreeNode);
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
	export class AppSetSummary {
	    key: string;
	    ctx: string;
	    name: string;
	    namespace: string;
	    problems?: Problem[];
	
	    static createFrom(source: any = {}) {
	        return new AppSetSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.ctx = source["ctx"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.problems = this.convertValues(source["problems"], Problem);
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
	export class AppSetDetail {
	    summary: AppSetSummary;
	    spec: string;
	    generators: string[];
	    conditions: argocd.Condition[];
	    apps: string[];
	    preserve: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppSetDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = this.convertValues(source["summary"], AppSetSummary);
	        this.spec = source["spec"];
	        this.generators = source["generators"];
	        this.conditions = this.convertValues(source["conditions"], argocd.Condition);
	        this.apps = source["apps"];
	        this.preserve = source["preserve"];
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
	export class AppSetGuard {
	    appSet: string;
	    applicationsSync: string;
	    ignored: string[];
	    loadError?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSetGuard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appSet = source["appSet"];
	        this.applicationsSync = source["applicationsSync"];
	        this.ignored = source["ignored"];
	        this.loadError = source["loadError"];
	    }
	}
	
	export class AppSources {
	    multi: boolean;
	    sources: any[];
	
	    static createFrom(source: any = {}) {
	        return new AppSources(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.multi = source["multi"];
	        this.sources = source["sources"];
	    }
	}
	
	export class ArgoConfig {
	    version: string;
	    repositories: any[];
	    projects: any[];
	    accounts: any[];
	    clusters: any[];
	    settings: string;
	    errors: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ArgoConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.repositories = source["repositories"];
	        this.projects = source["projects"];
	        this.accounts = source["accounts"];
	        this.clusters = source["clusters"];
	        this.settings = source["settings"];
	        this.errors = source["errors"];
	    }
	}
	export class ClusterSummary {
	    ctx: string;
	    name: string;
	    server: string;
	    state: string;
	    message?: string;
	    version?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClusterSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ctx = source["ctx"];
	        this.name = source["name"];
	        this.server = source["server"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.version = source["version"];
	    }
	}
	export class ContextStatus {
	    id: string;
	    name: string;
	    server: string;
	    authType: string;
	    color: string;
	    disabled: boolean;
	    state: string;
	    message?: string;
	    version?: string;
	    user?: string;
	    synced?: string;
	    cachedAt?: string;
	    appSetsError?: string;
	    clustersError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContextStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.server = source["server"];
	        this.authType = source["authType"];
	        this.color = source["color"];
	        this.disabled = source["disabled"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.version = source["version"];
	        this.user = source["user"];
	        this.synced = source["synced"];
	        this.cachedAt = source["cachedAt"];
	        this.appSetsError = source["appSetsError"];
	        this.clustersError = source["clustersError"];
	    }
	}
	export class DeleteOptions {
	    cascade: boolean;
	    policy: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cascade = source["cascade"];
	        this.policy = source["policy"];
	    }
	}
	export class DiffItem {
	    group: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    modified: boolean;
	    hook: boolean;
	    live: string;
	    target: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group = source["group"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.modified = source["modified"];
	        this.hook = source["hook"];
	        this.live = source["live"];
	        this.target = source["target"];
	    }
	}
	export class EventRow {
	    type: string;
	    reason: string;
	    message: string;
	    count: number;
	    first: string;
	    last: string;
	    object: string;
	    component: string;
	
	    static createFrom(source: any = {}) {
	        return new EventRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.reason = source["reason"];
	        this.message = source["message"];
	        this.count = source["count"];
	        this.first = source["first"];
	        this.last = source["last"];
	        this.object = source["object"];
	        this.component = source["component"];
	    }
	}
	export class HistoryEntry {
	    id: number;
	    revision: string;
	    deployedAt: string;
	    startedAt?: string;
	    source?: string;
	    path?: string;
	    target?: string;
	    author?: string;
	    date?: string;
	    message?: string;
	    metaError?: string;
	    current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.revision = source["revision"];
	        this.deployedAt = source["deployedAt"];
	        this.startedAt = source["startedAt"];
	        this.source = source["source"];
	        this.path = source["path"];
	        this.target = source["target"];
	        this.author = source["author"];
	        this.date = source["date"];
	        this.message = source["message"];
	        this.metaError = source["metaError"];
	        this.current = source["current"];
	    }
	}
	export class ImageUpdater {
	    enabled: boolean;
	    annotations: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ImageUpdater(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.annotations = source["annotations"];
	    }
	}
	export class LogRequest {
	    group: string;
	    version: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    container: string;
	    tailLines: number;
	    follow: boolean;
	    previous: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LogRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group = source["group"];
	        this.version = source["version"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.container = source["container"];
	        this.tailLines = source["tailLines"];
	        this.follow = source["follow"];
	        this.previous = source["previous"];
	    }
	}
	
	
	export class SyncPolicy {
	    automated: boolean;
	    prune: boolean;
	    selfHeal: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SyncPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.automated = source["automated"];
	        this.prune = source["prune"];
	        this.selfHeal = source["selfHeal"];
	    }
	}
	export class TerminalRequest {
	    namespace: string;
	    pod: string;
	    container: string;
	
	    static createFrom(source: any = {}) {
	        return new TerminalRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.pod = source["pod"];
	        this.container = source["container"];
	    }
	}

}

export namespace updater {
	
	export class Release {
	    tag: string;
	    version: string;
	    url: string;
	    // Go type: time
	    publishedAt: any;
	    notes: string;
	    assetUrl: string;
	    assetName: string;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag = source["tag"];
	        this.version = source["version"];
	        this.url = source["url"];
	        this.publishedAt = this.convertValues(source["publishedAt"], null);
	        this.notes = source["notes"];
	        this.assetUrl = source["assetUrl"];
	        this.assetName = source["assetName"];
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

