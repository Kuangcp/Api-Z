package app

// Sync repository format 2: plaintext collection structure plus encrypted
// settings and one encrypted file per request.
type syncStructure struct {
	FormatVersion int              `json:"formatVersion"`
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Folders       []syncFolderRef  `json:"folders"`
	Requests      []syncRequestRef `json:"requests"`
}

type syncFolderRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parentID,omitempty"`
	SortOrder int    `json:"sortOrder"`
}

type syncRequestRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FolderID  string `json:"folderID,omitempty"`
	SortOrder int    `json:"sortOrder"`
}

type syncSettings struct {
	CollectionMeta string            `json:"collectionMeta"`
	FolderMeta     map[string]string `json:"folderMeta,omitempty"`
}

type syncRequestFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	HeadersText string `json:"headersText"`
	ParamsText  string `json:"paramsText"`
	BodyText    string `json:"bodyText"`
	MetaJSON    string `json:"metaJSON"`
}
