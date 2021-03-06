package gui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/jesseduffield/lazygit/pkg/utils"
	"github.com/spkg/bom"
)

var cyclableViews = []string{"status", "files", "branches", "commits", "stash"}

func (gui *Gui) refreshSidePanels(g *gocui.Gui) error {
	if err := gui.refreshBranches(g); err != nil {
		return err
	}
	if err := gui.refreshFiles(g); err != nil {
		return err
	}
	if err := gui.refreshCommits(g); err != nil {
		return err
	}
	return gui.refreshStashEntries(g)
}

func (gui *Gui) nextView(g *gocui.Gui, v *gocui.View) error {
	var focusedViewName string
	if v == nil || v.Name() == cyclableViews[len(cyclableViews)-1] {
		focusedViewName = cyclableViews[0]
	} else {
		for i := range cyclableViews {
			if v.Name() == cyclableViews[i] {
				focusedViewName = cyclableViews[i+1]
				break
			}
			if i == len(cyclableViews)-1 {
				message := gui.Tr.TemplateLocalize(
					"IssntListOfViews",
					Teml{
						"name": v.Name(),
					},
				)
				gui.Log.Info(message)
				return nil
			}
		}
	}
	focusedView, err := g.View(focusedViewName)
	if err != nil {
		panic(err)
	}
	return gui.switchFocus(g, v, focusedView)
}

func (gui *Gui) previousView(g *gocui.Gui, v *gocui.View) error {
	var focusedViewName string
	if v == nil || v.Name() == cyclableViews[0] {
		focusedViewName = cyclableViews[len(cyclableViews)-1]
	} else {
		for i := range cyclableViews {
			if v.Name() == cyclableViews[i] {
				focusedViewName = cyclableViews[i-1] // TODO: make this work properly
				break
			}
			if i == len(cyclableViews)-1 {
				message := gui.Tr.TemplateLocalize(
					"IssntListOfViews",
					Teml{
						"name": v.Name(),
					},
				)
				gui.Log.Info(message)
				return nil
			}
		}
	}
	focusedView, err := g.View(focusedViewName)
	if err != nil {
		panic(err)
	}
	return gui.switchFocus(g, v, focusedView)
}

func (gui *Gui) newLineFocused(g *gocui.Gui, v *gocui.View) error {
	switch v.Name() {
	case "menu":
		return gui.handleMenuSelect(g, v)
	case "status":
		return gui.handleStatusSelect(g, v)
	case "files":
		return gui.handleFileSelect(g, v)
	case "branches":
		return gui.handleBranchSelect(g, v)
	case "commits":
		return gui.handleCommitSelect(g, v)
	case "stash":
		return gui.handleStashEntrySelect(g, v)
	case "confirmation":
		return nil
	case "commitMessage":
		return gui.handleCommitFocused(g, v)
	case "credentials":
		return gui.handleCredentialsViewFocused(g, v)
	case "main":
		// TODO: pull this out into a 'view focused' function
		gui.refreshMergePanel(g)
		v.Highlight = false
		return nil
	case "staging":
		return nil
		// return gui.handleStagingSelect(g, v)
	default:
		panic(gui.Tr.SLocalize("NoViewMachingNewLineFocusedSwitchStatement"))
	}
}

func (gui *Gui) returnFocus(g *gocui.Gui, v *gocui.View) error {
	previousView, err := g.View(gui.State.PreviousView)
	if err != nil {
		// always fall back to files view if there's no 'previous' view stored
		previousView, err = g.View("files")
		if err != nil {
			gui.Log.Error(err)
		}
	}
	return gui.switchFocus(g, v, previousView)
}

// pass in oldView = nil if you don't want to be able to return to your old view
func (gui *Gui) switchFocus(g *gocui.Gui, oldView, newView *gocui.View) error {
	// we assume we'll never want to return focus to a confirmation panel i.e.
	// we should never stack confirmation panels
	if oldView != nil && oldView.Name() != "confirmation" {
		oldView.Highlight = false
		message := gui.Tr.TemplateLocalize(
			"settingPreviewsViewTo",
			Teml{
				"oldViewName": oldView.Name(),
			},
		)
		gui.Log.Info(message)

		// second class panels should never have focus restored to them because
		// once they lose focus they are effectively 'destroyed'
		secondClassPanels := []string{"confirmation", "menu"}
		if !utils.IncludesString(secondClassPanels, oldView.Name()) {
			gui.State.PreviousView = oldView.Name()
		}
	}

	newView.Highlight = true
	message := gui.Tr.TemplateLocalize(
		"newFocusedViewIs",
		Teml{
			"newFocusedView": newView.Name(),
		},
	)
	gui.Log.Info(message)
	if _, err := g.SetCurrentView(newView.Name()); err != nil {
		return err
	}
	if _, err := g.SetViewOnTop(newView.Name()); err != nil {
		return err
	}

	g.Cursor = newView.Editable

	if err := gui.renderPanelOptions(); err != nil {
		return err
	}

	return gui.newLineFocused(g, newView)
}

func (gui *Gui) resetOrigin(v *gocui.View) error {
	if err := v.SetCursor(0, 0); err != nil {
		return err
	}
	return v.SetOrigin(0, 0)
}

// if the cursor down past the last item, move it to the last line
func (gui *Gui) focusPoint(cx int, cy int, v *gocui.View) error {
	if cy < 0 {
		return nil
	}
	ox, oy := v.Origin()
	_, height := v.Size()
	ly := height - 1

	// if line is above origin, move origin and set cursor to zero
	// if line is below origin + height, move origin and set cursor to max
	// otherwise set cursor to value - origin
	if ly > v.LinesHeight() {
		if err := v.SetCursor(cx, cy); err != nil {
			return err
		}
		if err := v.SetOrigin(ox, 0); err != nil {
			return err
		}
	} else if cy < oy {
		if err := v.SetCursor(cx, 0); err != nil {
			return err
		}
		if err := v.SetOrigin(ox, cy); err != nil {
			return err
		}
	} else if cy > oy+ly {
		if err := v.SetCursor(cx, ly); err != nil {
			return err
		}
		if err := v.SetOrigin(ox, cy-ly); err != nil {
			return err
		}
	} else {
		if err := v.SetCursor(cx, cy-oy); err != nil {
			return err
		}
	}
	return nil
}

func (gui *Gui) synchronousRenderString(g *gocui.Gui, viewName, s string) error {
	v, err := g.View(viewName)
	// just in case the view disappeared as this function was called, we'll
	// silently return if it's not found
	if err != nil {
		return nil
	}
	v.Clear()
	if err := v.SetOrigin(0, 0); err != nil {
		return err
	}
	output := string(bom.Clean([]byte(s)))
	output = utils.NormalizeLinefeeds(output)
	fmt.Fprint(v, output)
	return nil
}

func (gui *Gui) renderString(g *gocui.Gui, viewName, s string) error {
	g.Update(func(*gocui.Gui) error {
		return gui.synchronousRenderString(gui.g, viewName, s)
	})
	return nil
}

func (gui *Gui) optionsMapToString(optionsMap map[string]string) string {
	optionsArray := make([]string, 0)
	for key, description := range optionsMap {
		optionsArray = append(optionsArray, key+": "+description)
	}
	sort.Strings(optionsArray)
	return strings.Join(optionsArray, ", ")
}

func (gui *Gui) renderOptionsMap(optionsMap map[string]string) error {
	return gui.renderString(gui.g, "options", gui.optionsMapToString(optionsMap))
}

// TODO: refactor properly
// i'm so sorry but had to add this getBranchesView
func (gui *Gui) getFilesView(g *gocui.Gui) *gocui.View {
	v, _ := g.View("files")
	return v
}

func (gui *Gui) getCommitsView(g *gocui.Gui) *gocui.View {
	v, _ := g.View("commits")
	return v
}

func (gui *Gui) getCommitMessageView(g *gocui.Gui) *gocui.View {
	v, _ := g.View("commitMessage")
	return v
}

func (gui *Gui) getBranchesView(g *gocui.Gui) *gocui.View {
	v, _ := g.View("branches")
	return v
}

func (gui *Gui) getStagingView(g *gocui.Gui) *gocui.View {
	v, _ := g.View("staging")
	return v
}

func (gui *Gui) getMainView(g *gocui.Gui) *gocui.View {
	v, _ := g.View("main")
	return v
}

func (gui *Gui) getStashView(g *gocui.Gui) *gocui.View {
	v, _ := g.View("stash")
	return v
}

func (gui *Gui) trimmedContent(v *gocui.View) string {
	return strings.TrimSpace(v.Buffer())
}

func (gui *Gui) currentViewName(g *gocui.Gui) string {
	currentView := g.CurrentView()
	return currentView.Name()
}

func (gui *Gui) resizeCurrentPopupPanel(g *gocui.Gui) error {
	v := g.CurrentView()
	if v.Name() == "commitMessage" || v.Name() == "credentials" || v.Name() == "confirmation" {
		return gui.resizePopupPanel(g, v)
	}
	return nil
}

func (gui *Gui) resizePopupPanel(g *gocui.Gui, v *gocui.View) error {
	// If the confirmation panel is already displayed, just resize the width,
	// otherwise continue
	content := v.Buffer()
	x0, y0, x1, y1 := gui.getConfirmationPanelDimensions(g, content)
	vx0, vy0, vx1, vy1 := v.Dimensions()
	if vx0 == x0 && vy0 == y0 && vx1 == x1 && vy1 == y1 {
		return nil
	}
	gui.Log.Info(gui.Tr.SLocalize("resizingPopupPanel"))
	_, err := g.SetView(v.Name(), x0, y0, x1, y1, 0)
	return err
}

// generalFocusLine takes a lineNumber to focus, and a bottomLine to ensure we can see
func (gui *Gui) generalFocusLine(lineNumber int, bottomLine int, v *gocui.View) error {
	_, height := v.Size()
	overScroll := bottomLine - height + 1
	if overScroll < 0 {
		overScroll = 0
	}
	if err := v.SetOrigin(0, overScroll); err != nil {
		return err
	}
	if err := v.SetCursor(0, lineNumber-overScroll); err != nil {
		return err
	}
	return nil
}

func (gui *Gui) changeSelectedLine(line *int, total int, up bool) {
	if up {
		if *line == -1 || *line == 0 {
			return
		}

		*line -= 1
	} else {
		if *line == -1 || *line == total-1 {
			return
		}

		*line += 1
	}
}

func (gui *Gui) refreshSelectedLine(line *int, total int) {
	if *line == -1 && total > 0 {
		*line = 0
	} else if total-1 < *line {
		*line = total - 1
	}
}

func (gui *Gui) renderListPanel(v *gocui.View, items interface{}) error {
	gui.g.Update(func(g *gocui.Gui) error {
		list, err := utils.RenderList(items)
		if err != nil {
			return gui.createErrorPanel(gui.g, err.Error())
		}
		v.Clear()
		fmt.Fprint(v, list)
		return nil
	})
	return nil
}

func (gui *Gui) renderPanelOptions() error {
	currentView := gui.g.CurrentView()
	switch currentView.Name() {
	case "menu":
		return gui.renderMenuOptions()
	case "main":
		return gui.renderMergeOptions()
	default:
		return gui.renderGlobalOptions()
	}
}
