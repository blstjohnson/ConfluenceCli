package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xlab/treeprint"

	"confcli/pkg/api"
	"confcli/pkg/clients"
	"confcli/pkg/config"
	"confcli/pkg/converters"
	"confcli/pkg/formatters"
	"confcli/pkg/models"
	"confcli/pkg/transforms"
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
			dateStr, _ := cmd.Flags().GetString("date")
			scrollVersion, _ := cmd.Flags().GetString("scroll-version")
			transformProfile, _ := cmd.Flags().GetString("transform")
			setOverrides, _ := cmd.Flags().GetStringArray("set")

			// Resolve transform profile if specified
			var profile *transforms.TransformProfile
			if transformProfile != "" {
				var err error
				profile, err = transforms.ResolveProfile(transformProfile)
				if err != nil {
					return err
				}

				// Apply --set overrides
				if len(setOverrides) > 0 {
					overrideMap := make(map[string]string)
					for _, s := range setOverrides {
						parts := strings.SplitN(s, "=", 2)
						if len(parts) != 2 {
							return fmt.Errorf("invalid --set format %q: expected key=value", s)
						}
						overrideMap[parts[0]] = parts[1]
					}
					if err := transforms.ApplySetOverrides(profile, overrideMap); err != nil {
						return err
					}
				}

				// Apply profile folder settings as defaults (explicit flags override)
				if !cmd.Flags().Changed("named-folders") && !cmd.Flags().Changed("clean-names") {
					switch profile.Folder.Naming {
					case "slug":
						namedFolders = true
					case "title":
						cleanNames = true
					case "id":
						// default behavior
					}
				}
				if !cmd.Flags().Changed("no-length-limit") && profile.Folder.LengthLimit == 0 {
					noLengthLimit = true
				}
				if !cmd.Flags().Changed("flat-leaves") {
					flatLeaves = profile.Folder.FlatLeaves
				}
				// skip_root is now ONLY available as a CLI flag
				if !cmd.Flags().Changed("save-metadata") {
					saveMetadata = profile.Page.SaveMetadata
				}
				if !cmd.Flags().Changed("no-toc") {
					noTOC = profile.Page.StripTOC
				}
				if !cmd.Flags().Changed("format") && profile.Page.Format != "" {
					format = profile.Page.Format
				}
			}

			var dateFilter time.Time
			if dateStr != "" {
				var parseErr error
				// Try date-only format first (YYYY-MM-DD), treat as end of day
				dateFilter, parseErr = time.Parse("2006-01-02", dateStr)
				if parseErr != nil {
					// Try full datetime format
					dateFilter, parseErr = time.Parse(time.RFC3339, dateStr)
					if parseErr != nil {
						return fmt.Errorf("invalid date format %q: use YYYY-MM-DD or RFC3339 (e.g. 2024-01-15T14:30:00Z)", dateStr)
					}
				} else {
					// For date-only, use end of day so we include versions created on that date
					dateFilter = dateFilter.Add(24*time.Hour - time.Nanosecond)
				}
			}

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

				// Scroll Versions: detect plugin and resolve version-filtered page IDs
				var scrollPageIDs []int
				if scrollVersion != "" || outputDir != "" {
					svClient := apiClient.ScrollVersions()
					svCfg, svErr := svClient.GetConfig(ctx, space)
					if svErr != nil {
						fmt.Fprintf(os.Stderr, "Warning: could not probe Scroll Versions plugin: %v\n", svErr)
					}
					if svCfg != nil && svCfg.EnableVersionManagement {
						versions, vErr := svClient.GetVersions(ctx, space)
						if vErr != nil {
							fmt.Fprintf(os.Stderr, "Warning: could not list Scroll Versions: %v\n", vErr)
						} else if len(versions) > 0 {
							if scrollVersion != "" {
								// Find the requested version by name
								var targetVersion *models.ScrollVersion
								for i := range versions {
									if versions[i].Name == scrollVersion {
										targetVersion = &versions[i]
										break
									}
								}
								if targetVersion == nil {
									names := make([]string, len(versions))
									for i, v := range versions {
										names[i] = v.Name
									}
									return fmt.Errorf("scroll version %q not found; available versions: %s", scrollVersion, strings.Join(names, ", "))
								}

								// Walk the version-filtered page tree and resolve to Confluence page IDs
								resolvedIDs, resolveErr := resolveScrollVersionPages(ctx, svClient, apiClient, space, targetVersion.ID)
								if resolveErr != nil {
									return fmt.Errorf("failed to resolve scroll version pages: %w", resolveErr)
								}
								scrollPageIDs = resolvedIDs
								fmt.Fprintf(os.Stderr, "Scroll Versions: exporting version %q (%d pages)\n", scrollVersion, len(scrollPageIDs))
							} else {
								// No --scroll-version flag, but versions exist: inform the user
								names := make([]string, len(versions))
								for i, v := range versions {
									suffix := ""
									if v.Archived {
										suffix = " (archived)"
									}
									names[i] = v.Name + suffix
								}
								fmt.Fprintf(os.Stderr, "Note: this space uses Scroll Versions. Available versions: %s\n", strings.Join(names, ", "))
								fmt.Fprintf(os.Stderr, "Use --scroll-version=<name> to export a specific version.\n")
							}
						}
					}
				}

				if err := exportSpaceToDirectoryIterative(apiClient, space, pageID, outputDir, format, depth, skipContent, batchSize, rewriteLinks, rewriteTFSLinks, namedFolders, cleanNames, noLengthLimit, saveMetadata, flatLeaves, skipRoot, linkCfg, dateFilter, profile, scrollPageIDs); err != nil {
					return fmt.Errorf("failed to export space to directory: %w", err)
				}
				if scrollVersion != "" {
					fmt.Printf("Space %s (scroll version %q) exported to %s\n", space, scrollVersion, outputDir)
				} else if pageID > 0 {
					fmt.Printf("Page %d and descendants from space %s exported to %s\n", pageID, space, outputDir)
				} else {
					fmt.Printf("Space %s exported to %s\n", space, outputDir)
				}
				return nil
			}

			// For non-export commands (flat list, tree view) we need the hierarchy.
			outputFormat := viper.GetString("output_format")

			if pageID > 0 {
				// When a specific page is requested, fetch the root page + descendants
				// using the API client directly (same approach as exportSpaceToDirectoryIterative).
				rootPage, err := apiClient.GetPage(ctx, pageID)
				if err != nil {
					return fmt.Errorf("failed to get page %d: %w", pageID, err)
				}

				descendants, err := apiClient.GetDescendants(ctx, pageID, depth)
				if err != nil {
					return fmt.Errorf("failed to get descendants for page %d: %w", pageID, err)
				}

				// Collect subtree only: target page + descendants (no ancestors)
				allPages := make([]models.Page, 0, 1+len(descendants))
				allPages = append(allPages, *rootPage)
				allPages = append(allPages, descendants...)

				if flat {
					if outputFormat == "json" {
						flatPages := make([]map[string]interface{}, len(allPages))
						for i, page := range allPages {
							flatPages[i] = map[string]interface{}{
								"id":    page.ID,
								"title": page.Title,
							}
						}
						return formatters.FormatOutput(flatPages, "json")
					}
					return formatters.FormatOutput(allPages, "text")
				}

				// Build a subtree rooted at the target page (no ancestor spine).
				tree := treeprint.New()

				// Build children map from descendants.
				descendantChildrenMap := make(map[int][]*models.Page)
				for i := range descendants {
					d := &descendants[i]
					if len(d.Ancestors) > 0 {
						parentID, ok := d.Ancestors[len(d.Ancestors)-1].ID.Int()
						if ok {
							descendantChildrenMap[parentID] = append(descendantChildrenMap[parentID], d)
						}
					} else {
						// Descendant with no ancestors — treat as direct child of target page
						descendantChildrenMap[pageID] = append(descendantChildrenMap[pageID], d)
					}
				}

				// Add the target page as the tree root
				targetID, _ := rootPage.ID.Int()
				targetBranch := tree.AddBranch(fmt.Sprintf("%d: %s", targetID, rootPage.Title))

				// Attach descendant subtree under the target page
				buildTree(targetBranch, descendantChildrenMap[targetID], descendantChildrenMap, depth, 0)

				fmt.Println(tree.String())
			} else {
				// Full space hierarchy
				spaceUseCase := usecases.NewSpaceUseCase(apiClient)
				resp, err := spaceUseCase.GetSpaceHierarchy(ctx, &usecases.GetSpaceHierarchyRequest{
					SpaceKey: space,
					Depth:    depth,
					Flat:     flat,
				})
				if err != nil {
					return fmt.Errorf("failed to get space hierarchy: %w", err)
				}

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
			}

			return nil
		},
	}

	cmd.Flags().String("space", "", "Space key (required)")
	cmd.Flags().Int("page-id", 0, "Starting page ID (optional; downloads only the subtree rooted at this page)")
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
	cmd.Flags().String("date", "", "Retrieve page versions valid at this date (YYYY-MM-DD or RFC3339)")
	cmd.Flags().String("scroll-version", "", "Export a specific Scroll Versions version by name (requires Scroll Versions plugin)")
	cmd.Flags().String("transform", "", "Transform profile name or file path")
	cmd.Flags().StringArray("set", nil, "Override profile values (repeatable): key=value, e.g. --set page.strip_toc=true")
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
				if converters.HasPlantUMLImages(apiContent) {
					storageCnt, sErr := apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), "storage", 0)
					if sErr == nil {
						blocks := converters.ExtractPlantUMLBlocks(storageCnt)
						if len(blocks) > 0 {
							md, mdErr := converters.ExportViewToMarkdownKeepImages(apiContent, baseURL)
							if mdErr == nil {
								md = converters.ReplacePlantUMLImages(md, blocks)
								content = converters.StripJunkImages(md)
								err = nil
							} else {
								err = mdErr
							}
						}
					}
					if content == "" {
						content, err = converters.ExportViewToMarkdown(apiContent, baseURL)
					}
				} else {
					content, err = converters.ExportViewToMarkdown(apiContent, baseURL)
				}
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
func exportSpaceToDirectoryIterative(apiClient api.Client, space string, rootPageID int, outputDir, format string, depth int, skipContent bool, batchSize int, rewriteLinks bool, rewriteTFSLinks bool, namedFolders bool, cleanNames bool, noLengthLimit bool, saveMetadata bool, flatLeaves bool, skipRoot bool, linkCfg *converters.LinkRewriteConfig, dateFilter time.Time, profile *transforms.TransformProfile, scrollPageIDs []int) error {
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

	// Progress bar for page collection (total unknown, use spinner)
	collectBar := progressbar.NewOptions(-1,
		progressbar.OptionSetDescription("Collecting pages"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionShowCount(),
	)

	// Use iterative processing to fetch all pages
	var err error
	if len(scrollPageIDs) > 0 {
		// Scroll Versions mode: fetch each resolved Confluence page by ID.
		// scrollPageIDs is already the flat list of real Confluence page IDs in
		// tree order (parents before children) produced by resolveScrollVersionPages.
		scrollIDSet := make(map[int]bool, len(scrollPageIDs))
		for _, id := range scrollPageIDs {
			scrollIDSet[id] = true
		}
		for _, id := range scrollPageIDs {
			page, fetchErr := apiClient.GetPage(ctx, id)
			if fetchErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to fetch scroll-resolved page %d: %v\n", id, fetchErr)
				continue
			}
			pageCount++
			collectBar.Add(1)
			pID, ok := page.ID.Int()
			if !ok {
				continue
			}
			pageMap[pID] = page
			// Build parent-child: use ancestors, but only if parent is also in the scroll set
			if len(page.Ancestors) > 0 {
				parentID, pOk := page.Ancestors[len(page.Ancestors)-1].ID.Int()
				if pOk && scrollIDSet[parentID] {
					childrenMap[parentID] = append(childrenMap[parentID], pID)
				} else {
					rootPageIDs = append(rootPageIDs, pID)
				}
			} else {
				rootPageIDs = append(rootPageIDs, pID)
			}
		}
	} else if rootPageID > 0 {
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
			collectBar.Add(1)
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
				collectBar.Add(1)
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

	collectBar.Finish()
	fmt.Fprintln(os.Stderr)

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
		var buildFileMap func(pageID int, parentDir string, inheritedFlatten bool)
		buildFileMap = func(pageID int, parentDir string, inheritedFlatten bool) {
			page, exists := pageMap[pageID]
			if !exists {
				return
			}

			// Check skip/skip_content from transform profile
			if profile != nil {
				_, skip, skipCont := profile.ResolvePageConfig(pageID, "")
				if skip {
					// Skip entire subtree: no file map entries for page or descendants
					return
				}
				if skipCont {
					// Skip content file, but build folder path for children
					childIDs := childrenMap[pageID]
					if len(childIDs) > 0 {
						pgDir := filepath.Join(parentDir, getFolderName(pageID))
						for _, childID := range childIDs {
							buildFileMap(childID, pgDir, inheritedFlatten)
						}
					}
					return
				}
			}

			// Hierarchical flatten: if inherited, all descendants go into parentDir.
			// Only check profile/flatLeaves when not already inherited.
			effectiveFlatten := inheritedFlatten
			if !inheritedFlatten {
				effectiveFlatten = flatLeaves
				if profile != nil {
					effectiveFlatten = profile.ResolveFlatten(pageID, "", flatLeaves)
				}
			}
			var pageDir string
			if effectiveFlatten {
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
				buildFileMap(childID, pageDir, effectiveFlatten)
			}
		}
		for _, rootID := range rootPageIDs {
			if skipRoot {
				// Root page is transparent: its children start at parentDir=""
				for _, childID := range childrenMap[rootID] {
					buildFileMap(childID, "", false)
				}
			} else {
				buildFileMap(rootID, "", false)
			}
		}
		linkCfg.PageMap = pageFileMap
	}

	// Set up tiny URL expander (shared across all pages for caching)
	var tinyURLExpander *transforms.ExpandTinyURLs
	if rewriteLinks && linkCfg.ConfBaseURL != "" {
		resolver := transforms.CachingResolver(
			transforms.VerifyingResolver(func(id int) bool {
				if _, ok := pageMap[id]; ok {
					return true
				}
				// Page not in export — verify existence via Confluence API
				_, apiErr := apiClient.GetPage(ctx, id)
				return apiErr == nil
			}),
		)
		tinyURLExpander = transforms.NewExpandTinyURLs(linkCfg.ConfBaseURL, resolver)
	}

	// Progress bar for content download phase (total known)
	downloadBar := progressbar.NewOptions(pageCount,
		progressbar.OptionSetDescription("Downloading content"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
	)

	// Recursive function to save page and its children in hierarchy
	// parentDir is the absolute directory of the parent page (or spaceDir for root pages)
	var savePageWithChildren func(pageID int, parentDir string, currentDepth int, inheritedFlatten bool) error
	var downloadErrors []string
	savePageWithChildren = func(pageID int, parentDir string, currentDepth int, inheritedFlatten bool) error {
		page, exists := pageMap[pageID]
		if !exists {
			return nil
		}

		if depth > 0 && currentDepth >= depth {
			// Skip if max depth reached
			return nil
		}

		// Check if this page should be skipped based on transform profile
		// Do this BEFORE creating any directories to avoid empty folders
		if profile != nil {
			_, skip, skipCont := profile.ResolvePageConfig(pageID, "")
			if skip {
				// Skip entire subtree: page + all descendants, no folder, no children
				var countSubtree func(id int) int
				countSubtree = func(id int) int {
					n := 1
					for _, cID := range childrenMap[id] {
						n += countSubtree(cID)
					}
					return n
				}
				downloadBar.Add(countSubtree(pageID))
				return nil
			}
			if skipCont {
				// Skip content file only, still process children
				downloadBar.Add(1)
				childIDs := childrenMap[pageID]
				if len(childIDs) > 0 {
					// Create folder so children have a container
					pgDir := filepath.Join(parentDir, getFolderName(pageID))
					if err := os.MkdirAll(pgDir, 0755); err != nil {
						return fmt.Errorf("failed to create directory for page %d: %w", pageID, err)
					}
					for _, childID := range childIDs {
						if err := savePageWithChildren(childID, pgDir, currentDepth+1, inheritedFlatten); err != nil {
							return err
						}
					}
				}
				return nil
			}
		}

		// Hierarchical flatten: if inherited, all descendants go into parentDir.
		// Only check profile/flatLeaves when not already inherited.
		effectiveFlatten := inheritedFlatten
		if !inheritedFlatten {
			effectiveFlatten = flatLeaves
			if profile != nil {
				effectiveFlatten = profile.ResolveFlatten(pageID, "", flatLeaves)
			}
		}
		var pageDir string
		if effectiveFlatten {
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
				msg := fmt.Sprintf("Warning: failed to save metadata for page %d (%s): %v", pageID, page.Title, err)
				fmt.Fprintln(os.Stderr, msg)
				downloadErrors = append(downloadErrors, msg)
			}
		}

		if !skipContent {
			// Get content format - use export_view for export format
			apiFormat := "export_view"
			if normalizedFormat != "export" {
				apiFormat = utils.GetContentFormatForAPI(normalizedFormat)
			}

			// Determine which version to fetch
			fetchVersion := 0 // 0 means current
			if !dateFilter.IsZero() {
				// Resolve the version that was current at the given date
				versions, verErr := apiClient.GetPageVersions(ctx, pageID)
				if verErr != nil {
					msg := fmt.Sprintf("Warning: failed to get version history for page %d (%s), skipping: %v", pageID, page.Title, verErr)
					fmt.Fprintln(os.Stderr, msg)
					downloadErrors = append(downloadErrors, msg)
					downloadBar.Add(1)
					childIDs := childrenMap[pageID]
					for _, childID := range childIDs {
						if err := savePageWithChildren(childID, pageDir, currentDepth+1, effectiveFlatten); err != nil {
							return err
						}
					}
					return nil
				}
				fetchVersion = resolveVersionAtDate(versions, dateFilter)
				if fetchVersion == 0 {
					fmt.Fprintf(os.Stderr, "Warning: no version of page %d existed before %s, skipping\n", pageID, dateFilter.Format("2006-01-02"))
					// Skip to children
					childIDs := childrenMap[pageID]
					for _, childID := range childIDs {
						if err := savePageWithChildren(childID, pageDir, currentDepth+1, effectiveFlatten); err != nil {
							return err
						}
					}
					return nil
				}
			}

			// Try to get content from the page body first (already fetched via expansions)
			// but only when not using date-based version (cached body is always current version)
			var apiContent string
			if fetchVersion == 0 && page.Body != nil {
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

			// If still no content, fetch it via API call (with version if date-based)
			if apiContent == "" {
				var fetchErr error
				apiContent, fetchErr = apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), apiFormat, fetchVersion)
				if fetchErr != nil {
					msg := fmt.Sprintf("Warning: failed to get content for page %d (%s), skipping: %v", pageID, page.Title, fetchErr)
					fmt.Fprintln(os.Stderr, msg)
					downloadErrors = append(downloadErrors, msg)
					downloadBar.Add(1)
					childIDs := childrenMap[pageID]
					for _, childID := range childIDs {
						if err := savePageWithChildren(childID, pageDir, currentDepth+1, effectiveFlatten); err != nil {
							return err
						}
					}
					return nil
				}
			}

			// Expand Confluence tiny URLs (/x/AbCd) to canonical page URLs
			// before conversion so the link rewriter can match them.
			if tinyURLExpander != nil {
				tctx := &transforms.TransformContext{
					PreContent: apiContent,
					PageID:     pageID,
					PageTitle:  page.Title,
				}
				if err := tinyURLExpander.Apply(tctx); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: tiny URL expansion failed for page %d: %v\n", pageID, err)
				} else {
					apiContent = tctx.PreContent
				}
			}

			// Run pre-conversion transforms if profile is set
			if profile != nil {
				pageCfg, _, _ := profile.ResolvePageConfig(pageID, "")
				if len(pageCfg.Transforms) > 0 {
					reg := transforms.DefaultRegistry()
					pipeline, pipeErr := transforms.BuildPipeline(pageCfg.Transforms, reg)
					if pipeErr != nil {
						msg := fmt.Sprintf("Warning: failed to build transform pipeline for page %d (%s): %v", pageID, page.Title, pipeErr)
						fmt.Fprintln(os.Stderr, msg)
						downloadErrors = append(downloadErrors, msg)
					} else {
						tctx := &transforms.TransformContext{
							PreContent: apiContent,
							PageID:     pageID,
							PageTitle:  page.Title,
							Format:     normalizedFormat,
						}
						if err := pipeline.Run(tctx); err != nil {
							msg := fmt.Sprintf("Warning: pre-transform failed for page %d (%s): %v", pageID, page.Title, err)
							fmt.Fprintln(os.Stderr, msg)
							downloadErrors = append(downloadErrors, msg)
						} else {
							apiContent = tctx.PreContent
						}
					}
				}
			}

			var content string
			var convertErr error
			// For export/export_view format, convert to markdown
			if normalizedFormat == "export" {
				// Check for PlantUML images and dual-fetch storage format if needed
				if converters.HasPlantUMLImages(apiContent) {
					storageContent, storageErr := apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), "storage", fetchVersion)
					if storageErr == nil {
						blocks := converters.ExtractPlantUMLBlocks(storageContent)
						if len(blocks) > 0 {
							md, mdErr := converters.ExportViewToMarkdownKeepImages(apiContent, baseURL)
							if mdErr == nil {
								md = converters.ReplacePlantUMLImages(md, blocks)
								content = converters.StripJunkImages(md)
								convertErr = nil
							} else {
								convertErr = mdErr
							}
						}
					}
					if content == "" {
						content, convertErr = converters.ExportViewToMarkdown(apiContent, baseURL)
					}
				} else {
					content, convertErr = converters.ExportViewToMarkdown(apiContent, baseURL)
				}
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

			// Run post-conversion transforms if profile is set
			if profile != nil {
				pageCfg, _, _ := profile.ResolvePageConfig(pageID, "")
				if len(pageCfg.Transforms) > 0 {
					reg := transforms.DefaultRegistry()
					pipeline, pipeErr := transforms.BuildPipeline(pageCfg.Transforms, reg)
					if pipeErr == nil {
						tctx := &transforms.TransformContext{
							PreContent:  apiContent,
							PostContent: content,
							PageID:      pageID,
							PageTitle:   page.Title,
							Format:      normalizedFormat,
						}
						if err := pipeline.Run(tctx); err != nil {
							fmt.Fprintf(os.Stderr, "Warning: post-transform failed for page %d: %v\n", pageID, err)
						} else {
							content = tctx.PostContent
						}
					}
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
				msg := fmt.Sprintf("Warning: failed to write page %d (%s) to file: %v", pageID, page.Title, writeErr)
				fmt.Fprintln(os.Stderr, msg)
				downloadErrors = append(downloadErrors, msg)
			}
		}

		downloadBar.Add(1)

		// Recursively save children
		childIDs := childrenMap[pageID]
		for _, childID := range childIDs {
			if err := savePageWithChildren(childID, pageDir, currentDepth+1, effectiveFlatten); err != nil {
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
				if err := savePageWithChildren(childID, spaceDir, 0, false); err != nil {
					return err
				}
			}
		} else {
			if err := savePageWithChildren(rID, spaceDir, 0, false); err != nil {
				return err
			}
		}
	}

	downloadBar.Finish()
	fmt.Fprintln(os.Stderr)

	if len(downloadErrors) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d page(s) had errors during export (see warnings above)\n", len(downloadErrors))
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

// resolveScrollVersionPages walks the Scroll Versions page tree for the given
// versionID and resolves every scroll page to its real Confluence page ID.
// Returns a flat list of Confluence page IDs (parents before children).
func resolveScrollVersionPages(ctx context.Context, svClient *api.ScrollVersionsClient, apiClient api.Client, spaceKey, versionID string) ([]int, error) {
	// Get top-level page tree nodes
	roots, err := svClient.GetPageTree(ctx, spaceKey, versionID)
	if err != nil {
		return nil, err
	}

	var result []int

	// Recursive walk: for each node, resolve its Confluence page, then recurse into children.
	var walk func(nodes []models.ScrollPageTreeNode) error
	walk = func(nodes []models.ScrollPageTreeNode) error {
		for _, node := range nodes {
			if node.ScrollPageID == "" || node.IsDeleted {
				continue
			}

			// If the tree node already has a changePageId (the version-specific
			// Confluence page), use it directly.
			if node.ChangePageID > 0 {
				result = append(result, int(node.ChangePageID))
			} else if node.ID > 0 {
				// Fallback: use the node's base Confluence page ID
				result = append(result, int(node.ID))
			} else {
				// Must resolve via the page endpoint
				sp, resolveErr := svClient.ResolvePage(ctx, spaceKey, node.ScrollPageID, versionID)
				if resolveErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to resolve scroll page %s: %v\n", node.ScrollPageID, resolveErr)
					continue
				}
				if sp == nil || sp.ConfluencePage == nil {
					continue
				}
				result = append(result, int(sp.ConfluencePage.ID))
			}

			// If the node reports children but the children slice is empty,
			// fetch them explicitly via the pagetree children endpoint.
			children := node.Children
			if node.HasChildren && len(children) == 0 {
				fetched, fetchErr := svClient.GetPageTreeChildren(ctx, spaceKey, versionID, node.ScrollPageID, node.ID)
				if fetchErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to fetch children for scroll page %s: %v\n", node.ScrollPageID, fetchErr)
				} else {
					children = fetched
				}
			}

			if len(children) > 0 {
				if err := walk(children); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := walk(roots); err != nil {
		return nil, err
	}
	return result, nil
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

// resolveVersionAtDate finds the version number that was current at the given date.
// It returns the latest version with UpdatedAt <= date, or 0 if no version qualifies.
func resolveVersionAtDate(versions []models.Version, date time.Time) int {
	bestVersion := 0
	var bestTime time.Time
	for _, v := range versions {
		if !v.UpdatedAt.IsZero() && !v.UpdatedAt.After(date) {
			if v.UpdatedAt.After(bestTime) || bestVersion == 0 {
				bestVersion = v.Number
				bestTime = v.UpdatedAt
			}
		}
	}
	return bestVersion
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
