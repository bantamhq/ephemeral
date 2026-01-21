package tui

const (
	footerHeight = 1
	headerHeight = 1
	repoPageSize = 50

	nameMaxLength = 128

	contentPaddingWidth = 6
	contentMinWidth     = 30
	columnMinWidth      = 10
	columnSplitFactor   = 4
	columnGapWidth      = 2

	listHeaderHeight = 2
	repoItemHeight   = 3

	repoLoadMoreThreshold = 5

	detailInfoBaseRows          = 0
	detailTabHeaderRows         = 3
	detailTabContentPaddingRows = 0
	detailTabBottomBorderRows   = 1
	detailTabFrameRows          = detailTabHeaderRows + detailTabContentPaddingRows + detailTabBottomBorderRows
	detailViewportOverhead      = detailInfoBaseRows + detailTabFrameRows
	detailViewportBorderWidth   = 2
	detailTabMinHeight          = 4
	detailViewportTopPadding    = 0
	detailViewportBottomPadding = 1

	detailCommitsLimit = 20

	shortSHAWidth           = 7
	tabContentBorderWidth   = 2
	tabScrollIndicatorPadding = 2

	dialogWidth          = 40
	dialogInputWidth     = 30
	helpDialogMinWidth   = 40
	helpDialogMaxWidth   = 80
	folderPickerWidth    = 40
	folderPickerHeight   = 15
	folderPickerMaxItems = 8
)

type layoutSizes struct {
	contentWidth int
	folderWidth  int
	repoWidth    int
	detailWidth  int
}

func (m Model) layoutSizes() layoutSizes {
	contentWidth := max(m.width-contentPaddingWidth, contentMinWidth)
	folderWidth := max(contentWidth/columnSplitFactor, columnMinWidth)
	repoWidth := max(contentWidth/columnSplitFactor, columnMinWidth)
	detailWidth := max(contentWidth-folderWidth-repoWidth, columnMinWidth)

	return layoutSizes{
		contentWidth: contentWidth,
		folderWidth:  folderWidth,
		repoWidth:    repoWidth,
		detailWidth:  detailWidth,
	}
}

func listViewportHeight(height int) int {
	return max(height-listHeaderHeight, 1)
}

func (m Model) detailViewportHeight() int {
	return max(m.mainHeight()-detailViewportOverhead, 1)
}

func repoViewportHeight(height int) int {
	available := height - listHeaderHeight
	if available < repoItemHeight {
		return 1
	}
	return max(available/repoItemHeight, 1)
}

func detailTabContainerHeight(height int) int {
	containerHeight := height - detailInfoBaseRows
	if containerHeight < detailTabMinHeight {
		return detailTabMinHeight
	}
	return containerHeight
}
