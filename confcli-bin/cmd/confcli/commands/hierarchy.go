package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xlab/treeprint"

	"confcli/pkg/api"
	"confcli/pkg/clients"
	"confcli/pkg/config"
	"confcli/pkg/converters"
	"confcli/pkg/formatters"
	"confcli/pkg/models"
	"confcli/pkg/usecases"
	"confcli/pkg/utils"
)

// NewHierarchyCmd creates the hierarchy command
func NewHierarchyCmd() *cobra.Command {
	hierarchyCmd := &cobra.Command{
		Use:   "hierarchy",
		Short: "Commands for working with page hierarchies",
		Long:  `Commands for retrieving and exporting page hierarchies`,
	}

	hierarchyCmd.AddCommand(newHierarchySpaceCmd())

	return hierarchyCmd
}

// newHierarchySpaceCmd implements the hierarchy space command
func newHierarchySpaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "space",
		Short: "Get hierarchy of a space",
		Long:  "Retrieve and export the hierarchy of pages in a Confluence space",
		RunE: func(cmd *cobra.Command, args []string) error {
			space, _ := cmd.Flags().GetString("space")
			pageID, _ := cmd.Flags().GetInt("page-id")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			format, _ := cmd.Flags().GetString("format")
			depth, _ := cmd.Flags().GetInt("depth")
			flat, _ := cmd.Flags().GetBool("flat")
			treeView, _ := cmd.Flags().GetBool("tree")
			skipContent, _ := cmd.Flags().GetBool("skip-content")
			batchSize, _ := cmd.Flags().GetInt("batch-size")
			namedFolders, _ := cmd.Flags().GetBool("named-folders")
			cleanNames, _ := cmd.Flags().GetBool("clean-names")
			noLengthLimit, _ := cmd.Flags().GetBool("no-length-limit")
			saveMetadata, _ := cmd.Flags().GetBool("save-metadata")
			flatLeaves, _ := cmd.Flags().GetBool("flat-leaves")
			noTOC, _ := cmd.Flags().GetBool("no-toc")
			skipRoot, _ := cmd.Flags().GetBool("skip-root")
			rewriteLinks, _ := cmd.Flags().GetBool("rewrite-links")
			rewriteTFSLinks, _ := cmd.Flags().GetBool("rewrite-tfs-links")
			tfsBaseURL, _ := cmd.Flags().GetString("tfs-base-url")
			localRepoPath, _ := cmd.Flags().GetString("local-repo-path")

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// When exporting to a directory, skip the space hierarchy request entirely —
			// exportSpaceToDirectoryIterative fetches pages itself (iteratively or by pageID).
			if outputDir != "" {
				// Load TFS config from profile if not overridden by flags
				if rewriteTFSLinks && tfsBaseURL == "" {
					if cfg, err := config.LoadConfig(); err == nil {
						profile := cfg.Profiles[cfg.CurrentProfile]
						if profile != nil {
							if tfsBaseURL == "" {
								tfsBaseURL = profile.TFSBaseURL
							}
							if localRepoPath == "" {
								localRepoPath = profile.LocalRepoPath
							}
						}
					}
				}

				// Build a cached page-existence checker.
				// For every Confluence link that points outside the current export
				// we call the API once to verify the target page still exists and is
				// not trashed. Results are memoised to avoid redundant API requests.
				pageExistsCache := make(map[int]bool)
				pageExistsChecker := func(pageID int) bool {
					if exists, ok := pageExistsCache[pageID]; ok {
						return exists
					}
					page, err := apiClient.GetPage(ctx, pageID)
					exists := err == nil && page != nil &&
						page.Status != "trashed" && page.Status != "deleted"
					pageExistsCache[pageID] = exists
					return exists
				}

				linkCfg := &converters.LinkRewriteConfig{
					ConfBaseURL:       viper.GetString("url"),
					TFSBaseURL:        tfsBaseURL,
					LocalRepoPath:     localRepoPath,
					PageExistsChecker: pageExistsChecker,
				}

				// Apply package-level conversion settings (safe for CLI: single-threaded)
				converters.DisableTOC = noTOC

				if err := exportSpaceToDirectoryIterative(apiClient, space, pageID, outputDir, format, depth, skipContent, batchSize, rewriteLinks, rewriteTFSLinks, namedFolders, cleanNames, noLengthLimit, saveMetadata, flatLeaves, skipRoot, linkCfg); err != nil {
					return fmt.Errorf("failed to export space to directory: %w", err)
				}
				if pageID > 0 {
					fmt.Printf("Page %d and descendants from space %s exported to %s\n", pageID, space, outputDir)
				} else {
					fmt.Printf("Space %s exported to %s\n", space, outputDir)
				}
				return nil
			}

			// For non-export commands (flat list, tree view) we need the space hierarchy.
			spaceUseCase := usecases.NewSpaceUseCase(apiClient)
			resp, err := spaceUseCase.GetSpaceHierarchy(ctx, &usecases.GetSpaceHierarchyRequest{
				SpaceKey: space,
				Depth:    depth,
				Flat:     flat,
			})
			if err != nil {
				return fmt.Errorf("failed to get space hierarchy: %w", err)
			}

			outputFormat := viper.GetString("output_format")

			if flat {
				if outputFormat == "json" {
					flatPages := make([]map[string]interface{}, len(resp.AllPages))
					for i, page := range resp.AllPages {
						flatPages[i] = map[string]interface{}{
							"id":    page.ID,
							"title": page.Title,
							"depth": 0,
						}
					}
					return formatters.FormatOutput(flatPages, "json")
				}
				return formatters.FormatOutput(resp.AllPages, "text")
			} else if treeView {
				tree := treeprint.New()
				for _, page := range resp.RootPages {
					pID, _ := page.ID.Int()
					tree.AddBranch(fmt.Sprintf("%d: %s", pID, page.Title))
				}
				fmt.Println(tree.String())
			} else {
				tree := treeprint.New()
				pageMap := make(map[int]*models.Page)
				childrenMap := make(map[int][]*models.Page)

				for i := range resp.AllPages {
					page := &resp.AllPages[i]
					pID, ok := page.ID.Int()
					if ok {
						pageMap[pID] = page
						childrenMap[0] = append(childrenMap[0], page)
					}
				}
				_ = pageMap // used via childrenMap
				buildTree(tree, childrenMap[0], childrenMap, depth, 0)
				fmt.Println(tree.String())
			}

			return nil
		},
	}

	cmd.Flags().String("space", "", "Space key (required)")
	cmd.Flags().Int("page-id", 0, "Root page ID to export (optional, if set, only this page and its descendants will be exported)")
	cmd.Flags().String("output-dir", "", "Export space to directory")
	cmd.Flags().String("format", "markdown", "Format for saved pages: markdown, storage, html, plain, edit, export/export_view (converted to markdown)")
	cmd.Flags().Int("depth", 0, "Recursion depth (default: unlimited)")
	cmd.Flags().Bool("flat", false, "Output flat list instead of tree")
	cmd.Flags().Bool("tree", false, "Display ASCII tree in console")
	cmd.Flags().Bool("skip-content", false, "Export only structure and metadata, no content")
	cmd.Flags().Int("batch-size", 10, "Batch size for iterative page fetching")
	cmd.Flags().Bool("rewrite-links", true, "Rewrite Confluence internal links to relative file paths during export")
	cmd.Flags().Bool("rewrite-tfs-links", false, "Rewrite TFS/Git repository links to local paths")
	cmd.Flags().String("tfs-base-url", "", "TFS base URL for link rewriting (overrides config)")
	cmd.Flags().String("local-repo-path", "", "Local repo path prefix for TFS link rewriting (overrides config)")
	cmd.Flags().Bool("named-folders", false, "Use transliterated page names for folder names instead of page IDs")
	cmd.Flags().Bool("clean-names", false, "Use page titles as folder/file names: remove forbidden chars, replace dots and spaces with '_', no transliteration, no page ID prefix")
	cmd.Flags().Bool("no-length-limit", false, "Remove the 80-character limit on folder and file names")
	cmd.Flags().Bool("save-metadata", false, "Save per-page .meta.json and space _space_metadata.json files alongside content (disabled by default)")
	cmd.Flags().Bool("flat-leaves", false, "Do not create a subdirectory for pages that have no children; save their content file directly in the parent folder")
	cmd.Flags().Bool("no-toc", false, "Strip the table of contents from exported markdown (instead of regenerating it as a clean list)")
	cmd.Flags().Bool("skip-root", false, "Skip creating folder/file for root page(s); their children are exported directly into the output directory")
	cmd.MarkFlagRequired("space")

	return cmd
}

// exportSpaceToDirectory exports the space hierarchy to a directory structure
// It supports iterative batch processing to save memory for large spaces
func exportSpaceToDirectory(apiClient interface {
	GetPageContent(context.Context, interface{}, string, int) (string, error)
	GetSpace(context.Context, string) (*models.Space, error)
	GetAllPagesInSpace(context.Context, string) ([]models.Page, error)
}, space, outputDir, format string, depth int, skipContent bool) error {
	ctx := context.Background()

	pages, err := apiClient.GetAllPagesInSpace(ctx, space)
	if err != nil {
		return fmt.Errorf("failed to get pages in space: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	spaceDir := outputDir
	if err := os.MkdirAll(spaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create space directory: %w", err)
	}

	baseURL := viper.GetString("url")

	for _, page := range pages {
		pageID, _ := page.ID.Int()
		if !skipContent {
			// Get content format - Confluence supports "storage", "editor", and "export_view" formats
			apiFormat := utils.GetContentFormatForAPI(format)

			// Try to get content from the page body first (already fetched via expansions)
			var apiContent string
			if page.Body != nil {
				bodyKey := apiFormat
				if bodyKey == "editor" {
					bodyKey = "editor"
				} else if bodyKey == "export_view" {
					bodyKey = "export_view"
				} else {
					bodyKey = "storage"
				}

				if bodyContent, ok := page.Body[bodyKey].(map[string]interface{}); ok {
					if value, ok := bodyContent["value"].(string); ok && value != "" {
						apiContent = value
					}
				}

				// If not found in requested format, try other formats
				if apiContent == "" {
					for _, key := range []string{"storage", "editor", "export_view", "view"} {
						if bodyContent, ok := page.Body[key].(map[string]interface{}); ok {
							if value, ok := bodyContent["value"].(string); ok && value != "" {
								apiContent = value
								break
							}
						}
					}
				}
			}

			var content string
			var err error
			// If still no content, fetch it via API call
			if apiContent == "" {
				fmt.Fprintf(os.Stderr, "Warning: page %d body not available, fetching separately\n", pageID)
				apiContent, err = apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), apiFormat, 0)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to get content for page %d: %v\n", pageID, err)
					continue
				}
			}

			// For export_view format, convert to markdown if requested
			if format == "export" {
				// export_view is clean HTML, convert to markdown
				content, err = converters.ExportViewToMarkdown(apiContent, baseURL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to convert export_view to markdown for page %d: %v\n", pageID, err)
					content = apiContent
				}
			} else {
				// Convert from API format to requested format
				content, err = utils.ConvertContentFromStorage(apiContent, format, baseURL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to convert content for page %d: %v\n", pageID, err)
					content = apiContent
				}
			}

			filename := fmt.Sprintf("%d_%s.%s", pageID, utils.SanitizeFilename(page.Title), utils.GetExtensionForFormat(format))
			filePath := filepath.Join(spaceDir, filename)
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write page %d to file: %v\n", pageID, err)
			}
		}
	}

	spaceInfo, err := apiClient.GetSpace(ctx, space)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get space info: %v\n", err)
	} else {
		metadata := map[string]interface{}{
			"key":         spaceInfo.Key,
			"name":        spaceInfo.Name,
			"description": spaceInfo.Description,
			"homepageId":  spaceInfo.HomepageID,
			"status":      spaceInfo.Status,
		}

		metadataBytes, err := formatters.FormatOutputToString(metadata, "json")
		if err != nil {
			return fmt.Errorf("failed to format space metadata: %w", err)
		}

		metadataFile := filepath.Join(spaceDir, "_space_metadata.json")
		if err := os.WriteFile(metadataFile, []byte(metadataBytes), 0644); err != nil {
			return fmt.Errorf("failed to write space metadata: %w", err)
		}
	}

	return nil
}

// exportSpaceToDirectoryIterative exports the space hierarchy to a directory structure
// using iterative batch processing to save memory for large spaces
// Creates a hierarchical folder structure: each page gets a folder named by pageId
// Child pages are saved inside their parent page's folder
func exportSpaceToDirectoryIterative(apiClient api.Client, space string, rootPageID int, outputDir, format string, depth int, skipContent bool, batchSize int, rewriteLinks bool, rewriteTFSLinks bool, namedFolders bool, cleanNames bool, noLengthLimit bool, saveMetadata bool, flatLeaves bool, skipRoot bool, linkCfg *converters.LinkRewriteConfig) error {
	ctx := context.Background()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	spaceDir := outputDir
	if err := os.MkdirAll(spaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create space directory: %w", err)
	}

	baseURL := viper.GetString("url")
	pageCount := 0

	// Choose the sanitizer function based on flags.
	// cleanNames takes priority over namedFolders for the sanitization strategy.
	var sanitize func(string) string
	switch {
	case cleanNames && noLengthLimit:
		sanitize = utils.SanitizeFilenameSimpleNoLimit
	case cleanNames:
		sanitize = utils.SanitizeFilenameSimple
	case noLengthLimit:
		sanitize = utils.SanitizeFilenameNoLimit
	default:
		sanitize = utils.SanitizeFilename
	}

	// Normalize format - support both "export" and "export_view"
	normalizedFormat := format
	if format == "export_view" {
		normalizedFormat = "export"
	}

	// First pass: collect all pages and build parent-child relationships
	pageMap := make(map[int]*models.Page)
	childrenMap := make(map[int][]int) // parent ID -> list of child IDs
	var rootPageIDs []int

	// Use iterative processing to fetch all pages
	var err error
	if rootPageID > 0 {
		// Fetch specific page and its descendants
		// 1. Fetch the root page
		rootPage, err := apiClient.GetPage(ctx, rootPageID)
		if err != nil {
			return fmt.Errorf("failed to fetch root page %d: %w", rootPageID, err)
		}

		// 2. Fetch descendants
		// Note: GetDescendants might return a lot of pages, but for a single branch it should be manageable.
		// If it's too large, we might need a paginated GetDescendants in the client.
		// For now, we assume it fits in memory or the client handles pagination internally.
		descendants, err := apiClient.GetDescendants(ctx, rootPageID, depth)
		if err != nil {
			return fmt.Errorf("failed to fetch descendants for page %d: %w", rootPageID, err)
		}

		// Process root page
		allPages := append([]models.Page{*rootPage}, descendants...)

		for _, page := range allPages {
			pageCount++
			pageID, ok := page.ID.Int()
			if !ok {
				continue
			}

			pageMap[pageID] = &page

			// Build parent-child relationships
			// For the root page of our export, we treat it as a root (no parent in our context)
			if pageID == rootPageID {
				rootPageIDs = append(rootPageIDs, pageID)
			} else {
				// For descendants, find their parent
				if len(page.Ancestors) > 0 {
					// Get immediate parent (last ancestor)
					parentID, ok := page.Ancestors[len(page.Ancestors)-1].ID.Int()
					if ok {
						// Only add to childrenMap if parent is also in our export set
						// (which it should be, unless the tree is broken)
						// For partial export, we only care if the parent is in our map or is the root
						childrenMap[parentID] = append(childrenMap[parentID], pageID)
					}
				}
			}
		}

	} else {
		err = apiClient.GetAllPagesInSpaceIterative(ctx, space, batchSize, func(batch []models.Page) error {
			for _, page := range batch {
				pageCount++
				pageID, ok := page.ID.Int()
				if !ok {
					continue
				}

				pageMap[pageID] = &page

				// Build parent-child relationships
				if len(page.Ancestors) > 0 {
					// Get immediate parent (last ancestor)
					parentID, ok := page.Ancestors[len(page.Ancestors)-1].ID.Int()
					if ok {
						childrenMap[parentID] = append(childrenMap[parentID], pageID)
					}
				} else {
					// No ancestors = root page
					rootPageIDs = append(rootPageIDs, pageID)
				}
			}
			return nil
		})
	}

	if err != nil {
		return fmt.Errorf("failed to fetch pages: %w", err)
	}

	// Build folder name map for named-folders / clean-names mode.
	folderNameMap := make(map[int]string) // pageID -> folder name
	if namedFolders || cleanNames {
		if cleanNames {
			// clean-names: use simple sanitizer, never append page ID (even for duplicates)
			for _, rootID := range rootPageIDs {
				if page, ok := pageMap[rootID]; ok {
					folderNameMap[rootID] = sanitize(page.Title)
				}
			}
			for _, children := range childrenMap {
				for _, childID := range children {
					if page, ok := pageMap[childID]; ok {
						folderNameMap[childID] = sanitize(page.Title)
					}
				}
			}
		} else {
			// named-folders: transliterated names, page ID appended for duplicates
			siblingNames := make(map[int]map[string][]int) // parentID -> name -> list of pageIDs
			for _, rootID := range rootPageIDs {
				if page, ok := pageMap[rootID]; ok {
					name := sanitize(page.Title)
					if siblingNames[0] == nil {
						siblingNames[0] = make(map[string][]int)
					}
					siblingNames[0][name] = append(siblingNames[0][name], rootID)
				}
			}
			for parentID, children := range childrenMap {
				if siblingNames[parentID] == nil {
					siblingNames[parentID] = make(map[string][]int)
				}
				for _, childID := range children {
					if page, ok := pageMap[childID]; ok {
						name := sanitize(page.Title)
						siblingNames[parentID][name] = append(siblingNames[parentID][name], childID)
					}
				}
			}
			// Assign folder names, appending page ID for duplicates
			for _, nameMap := range siblingNames {
				for name, ids := range nameMap {
					if len(ids) == 1 {
						folderNameMap[ids[0]] = name
					} else {
						for _, id := range ids {
							folderNameMap[id] = fmt.Sprintf("%s-%d", name, id)
						}
					}
				}
			}
		}
	}

	// getFolderName returns the folder name for a page
	getFolderName := func(pageID int) string {
		if namedFolders || cleanNames {
			if name, ok := folderNameMap[pageID]; ok {
				return name
			}
		}
		return fmt.Sprintf("%d", pageID)
	}

	// Build page ID -> file path map for link rewriting
	// This maps each page ID to its content file path relative to spaceDir
	pageFileMap := make(map[int]string)
	if rewriteLinks {
		var buildFileMap func(pageID int, parentDir string)
		buildFileMap = func(pageID int, parentDir string) {
			page, exists := pageMap[pageID]
			if !exists {
				return
			}
			// When --flat-leaves: leaf pages (no children) live directly in the parent dir.
			isLeaf := len(childrenMap[pageID]) == 0
			var pageDir string
			if flatLeaves && isLeaf {
				pageDir = parentDir
			} else {
				pageDir = filepath.Join(parentDir, getFolderName(pageID))
			}
			var filename string
			if cleanNames {
				filename = fmt.Sprintf("%s.%s", sanitize(page.Title), utils.GetExtensionForFormat(normalizedFormat))
			} else {
				filename = fmt.Sprintf("%d_%s.%s", pageID, sanitize(page.Title), utils.GetExtensionForFormat(normalizedFormat))
			}
			pageFileMap[pageID] = filepath.ToSlash(filepath.Join(pageDir, filename))

			for _, childID := range childrenMap[pageID] {
				buildFileMap(childID, pageDir)
			}
		}
		for _, rootID := range rootPageIDs {
			if skipRoot {
				// Root page is transparent: its children start at parentDir=""
				for _, childID := range childrenMap[rootID] {
					buildFileMap(childID, "")
				}
			} else {
				buildFileMap(rootID, "")
			}
		}
		linkCfg.PageMap = pageFileMap
	}

	// Recursive function to save page and its children in hierarchy
	// parentDir is the absolute directory of the parent page (or spaceDir for root pages)
	var savePageWithChildren func(pageID int, parentDir string, currentDepth int) error
	savePageWithChildren = func(pageID int, parentDir string, currentDepth int) error {
		page, exists := pageMap[pageID]
		if !exists {
			return nil
		}

		if depth > 0 && currentDepth >= depth {
			// Skip if max depth reached
			return nil
		}

		// When --flat-leaves: leaf pages (no children) are saved directly in the parent
		// directory — no subdirectory is created for them.
		isLeaf := len(childrenMap[pageID]) == 0
		var pageDir string
		if flatLeaves && isLeaf {
			// parentDir already exists (created by the parent's MkdirAll, or is spaceDir).
			pageDir = parentDir
		} else {
			pageDir = filepath.Join(parentDir, getFolderName(pageID))
			if err := os.MkdirAll(pageDir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for page %d: %w", pageID, err)
			}
		}

		// Save page metadata (only when --save-metadata is set)
		if saveMetadata {
			if err := savePageMetadata(pageDir, page, sanitize, cleanNames); err != nil {
				return fmt.Errorf("failed to save metadata for page %d: %w", pageID, err)
			}
		}

		if !skipContent {
			// Get content format - use export_view for export format
			apiFormat := "export_view"
			if normalizedFormat != "export" {
				apiFormat = utils.GetContentFormatForAPI(normalizedFormat)
			}

			// Try to get content from the page body first (already fetched via expansions)
			var apiContent string
			if page.Body != nil {
				// First try the requested format
				if bodyContent, ok := page.Body[apiFormat].(map[string]interface{}); ok {
					if value, ok := bodyContent["value"].(string); ok && value != "" {
						apiContent = value
					}
				}

				// If not found, try export_view as fallback
				if apiContent == "" && apiFormat != "export_view" {
					if bodyContent, ok := page.Body["export_view"].(map[string]interface{}); ok {
						if value, ok := bodyContent["value"].(string); ok && value != "" {
							apiContent = value
							apiFormat = "export_view"
						}
					}
				}

				// If still not found, try storage as final fallback
				if apiContent == "" {
					if bodyContent, ok := page.Body["storage"].(map[string]interface{}); ok {
						if value, ok := bodyContent["value"].(string); ok && value != "" {
							apiContent = value
							apiFormat = "storage"
						}
					}
				}
			}

			// If still no content, fetch it via API call
			if apiContent == "" {
				var fetchErr error
				apiContent, fetchErr = apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), apiFormat, 0)
				if fetchErr != nil {
					return fmt.Errorf("failed to get content for page %d: %w", pageID, fetchErr)
				}
			}

			var content string
			var convertErr error
			// For export/export_view format, convert to markdown
			if normalizedFormat == "export" {
				// export_view is clean HTML, convert to markdown
				content, convertErr = converters.ExportViewToMarkdown(apiContent, baseURL)
				if convertErr != nil {
					content = apiContent
				}
			} else {
				// Convert from API format to requested format
				content, convertErr = utils.ConvertContentFromStorage(apiContent, normalizedFormat, baseURL)
				if convertErr != nil {
					content = apiContent
				}
			}

			var filename string
			if cleanNames {
				filename = fmt.Sprintf("%s.%s", sanitize(page.Title), utils.GetExtensionForFormat(normalizedFormat))
			} else {
				filename = fmt.Sprintf("%d_%s.%s", pageID, sanitize(page.Title), utils.GetExtensionForFormat(normalizedFormat))
			}
			absFilePath := filepath.Join(pageDir, filename)

			// Apply link rewriting if enabled
			if rewriteLinks || rewriteTFSLinks {
				// Compute current page's directory relative to spaceDir for relative path resolution
				currentRelDir := ""
				if relPath, ok := pageFileMap[pageID]; ok {
					currentRelDir = filepath.ToSlash(filepath.Dir(relPath))
				}
				rewriteCfg := &converters.LinkRewriteConfig{
					TFSBaseURL:      linkCfg.TFSBaseURL,
					LocalRepoPath:   linkCfg.LocalRepoPath,
					CurrentPageDir:  currentRelDir,
					CurrentFilePath: absFilePath,
				}
				if rewriteLinks && linkCfg.PageMap != nil {
					rewriteCfg.PageMap = linkCfg.PageMap
					rewriteCfg.ConfBaseURL = linkCfg.ConfBaseURL
				}
				if rewriteTFSLinks && linkCfg.TFSBaseURL != "" {
					rewriteCfg.TFSBaseURL = linkCfg.TFSBaseURL
				}
				// Propagate the page-existence checker so that Confluence links
				// pointing to deleted or missing pages are stripped during export.
				if rewriteLinks && linkCfg.PageExistsChecker != nil {
					rewriteCfg.PageExistsChecker = linkCfg.PageExistsChecker
				}
				content = converters.RewriteLinks(content, rewriteCfg)
			}
			if writeErr := os.WriteFile(absFilePath, []byte(content), 0644); writeErr != nil {
				return fmt.Errorf("failed to write page %d to file: %w", pageID, writeErr)
			}
		}

		// Recursively save children
		childIDs := childrenMap[pageID]
		for _, childID := range childIDs {
			if err := savePageWithChildren(childID, pageDir, currentDepth+1); err != nil {
				return err
			}
		}

		return nil
	}

	// Save all root pages (and their children recursively).
	// When --skip-root: the root page itself is not written; its children start at spaceDir.
	for _, rID := range rootPageIDs {
		if skipRoot {
			for _, childID := range childrenMap[rID] {
				if err := savePageWithChildren(childID, spaceDir, 0); err != nil {
					return err
				}
			}
		} else {
			if err := savePageWithChildren(rID, spaceDir, 0); err != nil {
				return err
			}
		}
	}

	// Write space metadata only when --save-metadata is set
	if saveMetadata {
		spaceInfo, err := apiClient.GetSpace(ctx, space)
		if err != nil {
			return fmt.Errorf("failed to get space info: %w", err)
		}

		metadata := map[string]interface{}{
			"key":         spaceInfo.Key,
			"name":        spaceInfo.Name,
			"description": spaceInfo.Description,
			"homepageId":  spaceInfo.HomepageID,
			"status":      spaceInfo.Status,
			"pageCount":   pageCount,
			"rootPages":   rootPageIDs,
		}

		metadataBytes, err := formatters.FormatOutputToString(metadata, "json")
		if err != nil {
			return fmt.Errorf("failed to format space metadata: %w", err)
		}

		metadataFile := filepath.Join(spaceDir, "_space_metadata.json")
		if err := os.WriteFile(metadataFile, []byte(metadataBytes), 0644); err != nil {
			return fmt.Errorf("failed to write space metadata: %w", err)
		}
	}

	return nil
}

// savePageMetadata saves page metadata to a JSON file.
// sanitize is the active filename sanitizer; cleanNames controls whether to
// omit the numeric page-ID prefix from the metadata filename.
func savePageMetadata(spaceDir string, page *models.Page, sanitize func(string) string, cleanNames bool) error {
	pageID, ok := page.ID.Int()
	if !ok {
		return nil
	}

	metadata := map[string]interface{}{
		"id":        pageID,
		"title":     page.Title,
		"type":      page.Type,
		"status":    page.Status,
		"version":   page.Version.Number,
		"createdAt": page.CreatedAt(),
		"updatedAt": page.UpdatedAt(),
	}

	// Add ancestors if present
	if len(page.Ancestors) > 0 {
		ancestorIDs := make([]int, len(page.Ancestors))
		for i, a := range page.Ancestors {
			if id, ok := a.ID.Int(); ok {
				ancestorIDs[i] = id
			}
		}
		metadata["ancestors"] = ancestorIDs
	}

	metadataBytes, err := formatters.FormatOutputToString(metadata, "json")
	if err != nil {
		return err
	}

	var filename string
	if cleanNames {
		filename = fmt.Sprintf("%s.meta.json", sanitize(page.Title))
	} else {
		filename = fmt.Sprintf("%d_%s.meta.json", pageID, sanitize(page.Title))
	}
	filePath := filepath.Join(spaceDir, filename)
	return os.WriteFile(filePath, []byte(metadataBytes), 0644)
}

// buildTree builds a tree structure from pages
func buildTree(tree treeprint.Tree, pages []*models.Page, childrenMap map[int][]*models.Page, maxDepth, currentDepth int) {
	if maxDepth > 0 && currentDepth >= maxDepth {
		return
	}

	for _, page := range pages {
		pageID, ok := page.ID.Int()
		if !ok {
			continue
		}

		branch := tree.AddBranch(fmt.Sprintf("%d: %s", pageID, page.Title))

		children := childrenMap[pageID]
		if len(children) > 0 {
			buildTree(branch, children, childrenMap, maxDepth, currentDepth+1)
		}
	}
}
