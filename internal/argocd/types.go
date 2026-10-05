package argocd

// Minimal subset of the Argo CD API types. Only the fields ArgoDeck reads are
// declared, which keeps decoding of very large application lists cheap.

type OwnerReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
	DeletionTimestamp *string           `json:"deletionTimestamp,omitempty"`
	OwnerReferences   []OwnerReference  `json:"ownerReferences,omitempty"`
}

type ListMeta struct {
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

type Application struct {
	Metadata  ObjectMeta `json:"metadata"`
	Spec      AppSpec    `json:"spec"`
	Status    AppStatus  `json:"status"`
	Operation *Operation `json:"operation,omitempty"`
}

type ApplicationList struct {
	Metadata ListMeta      `json:"metadata"`
	Items    []Application `json:"items"`
}

type Operation struct {
	Sync *struct {
		Revision string `json:"revision,omitempty"`
	} `json:"sync,omitempty"`
}

type AppSpec struct {
	Project     string      `json:"project"`
	Source      *AppSource  `json:"source,omitempty"`
	Sources     []AppSource `json:"sources,omitempty"`
	Destination Destination `json:"destination"`
	SyncPolicy  *SyncPolicy `json:"syncPolicy,omitempty"`
}

type AppSource struct {
	RepoURL        string `json:"repoURL"`
	Path           string `json:"path,omitempty"`
	TargetRevision string `json:"targetRevision,omitempty"`
	Chart          string `json:"chart,omitempty"`
	Ref            string `json:"ref,omitempty"`
}

type Destination struct {
	Server    string `json:"server,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

type SyncPolicy struct {
	Automated *struct {
		Prune    bool  `json:"prune,omitempty"`
		SelfHeal bool  `json:"selfHeal,omitempty"`
		Enabled  *bool `json:"enabled,omitempty"`
	} `json:"automated,omitempty"`
}

type AppStatus struct {
	Sync           SyncStatus        `json:"sync"`
	Health         HealthStatus      `json:"health"`
	OperationState *OperationState   `json:"operationState,omitempty"`
	Conditions     []Condition       `json:"conditions,omitempty"`
	Resources      []ResourceStatus  `json:"resources,omitempty"`
	ReconciledAt   string            `json:"reconciledAt,omitempty"`
	History        []RevisionHistory `json:"history,omitempty"`
}

type SyncStatus struct {
	Status    string   `json:"status"`
	Revision  string   `json:"revision,omitempty"`
	Revisions []string `json:"revisions,omitempty"`
}

type HealthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type OperationState struct {
	Phase      string      `json:"phase"`
	Message    string      `json:"message,omitempty"`
	StartedAt  string      `json:"startedAt,omitempty"`
	FinishedAt string      `json:"finishedAt,omitempty"`
	RetryCount int64       `json:"retryCount,omitempty"`
	SyncResult *SyncResult `json:"syncResult,omitempty"`
}

type SyncResult struct {
	Revision  string           `json:"revision,omitempty"`
	Resources []ResourceResult `json:"resources,omitempty"`
}

type ResourceResult struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
	Message   string `json:"message,omitempty"`
	HookPhase string `json:"hookPhase,omitempty"`
	HookType  string `json:"hookType,omitempty"`
	SyncPhase string `json:"syncPhase,omitempty"`
}

type Condition struct {
	Type               string `json:"type"`
	Message            string `json:"message"`
	Status             string `json:"status,omitempty"`
	Reason             string `json:"reason,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

type ResourceStatus struct {
	Group           string        `json:"group,omitempty"`
	Version         string        `json:"version"`
	Kind            string        `json:"kind"`
	Namespace       string        `json:"namespace,omitempty"`
	Name            string        `json:"name"`
	Status          string        `json:"status,omitempty"`
	Health          *HealthStatus `json:"health,omitempty"`
	Hook            bool          `json:"hook,omitempty"`
	RequiresPruning bool          `json:"requiresPruning,omitempty"`
}

type RevisionHistory struct {
	ID              int64      `json:"id"`
	Revision        string     `json:"revision,omitempty"`
	Revisions       []string   `json:"revisions,omitempty"`
	DeployedAt      string     `json:"deployedAt"`
	DeployStartedAt string     `json:"deployStartedAt,omitempty"`
	Source          *AppSource `json:"source,omitempty"`
}

type ApplicationWatchEvent struct {
	Type        string      `json:"type"`
	Application Application `json:"application"`
}

type ResourceTree struct {
	Nodes []ResourceNode `json:"nodes"`
}

type ResourceNode struct {
	Group      string        `json:"group,omitempty"`
	Version    string        `json:"version"`
	Kind       string        `json:"kind"`
	Namespace  string        `json:"namespace,omitempty"`
	Name       string        `json:"name"`
	UID        string        `json:"uid,omitempty"`
	ParentRefs []ResourceRef `json:"parentRefs,omitempty"`
	Health     *HealthStatus `json:"health,omitempty"`
	Info       []InfoItem    `json:"info,omitempty"`
	CreatedAt  string        `json:"createdAt,omitempty"`
}

type ResourceRef struct {
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

type InfoItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ApplicationSet struct {
	Metadata ObjectMeta `json:"metadata"`
	Status   struct {
		Conditions []Condition `json:"conditions,omitempty"`
	} `json:"status"`
}

type ApplicationSetList struct {
	Items []ApplicationSet `json:"items"`
}

type Cluster struct {
	Server          string `json:"server"`
	Name            string `json:"name"`
	ConnectionState struct {
		Status  string `json:"status"`
		Message string `json:"message,omitempty"`
	} `json:"connectionState"`
	Info struct {
		ConnectionState struct {
			Status  string `json:"status"`
			Message string `json:"message,omitempty"`
		} `json:"connectionState"`
		ServerVersion string `json:"serverVersion,omitempty"`
	} `json:"info"`
}

type ClusterList struct {
	Items []Cluster `json:"items"`
}

type Settings struct {
	URL        string `json:"url"`
	OIDCConfig *struct {
		Name        string   `json:"name"`
		Issuer      string   `json:"issuer"`
		ClientID    string   `json:"clientID"`
		CLIClientID string   `json:"cliClientID"`
		Scopes      []string `json:"scopes"`
	} `json:"oidcConfig,omitempty"`
	DexConfig *struct {
		Connectors []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"connectors"`
	} `json:"dexConfig,omitempty"`
}

type UserInfo struct {
	LoggedIn bool     `json:"loggedIn"`
	Username string   `json:"username"`
	Iss      string   `json:"iss"`
	Groups   []string `json:"groups"`
}

type Version struct {
	Version string `json:"Version"`
}
