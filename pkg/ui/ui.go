package ui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chandler767/Distributed-AI-Demo/pkg/agent"
	"github.com/chandler767/Distributed-AI-Demo/pkg/types"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// UI represents the terminal UI
type UI struct {
	app          *tview.Application
	messagesView *tview.TextView
	inputField   *tview.InputField
	messagesChan chan types.DebateMessage
	commandsChan chan string
	agent        *agent.Agent
	messagesLock sync.Mutex
	maxMessages  int
	messages     []types.DebateMessage
	lastDrawTime time.Time
	drawDebounce time.Duration
}

// NewUI creates a new terminal UI
func NewUI(agent *agent.Agent) *UI {
	ui := &UI{
		app:          tview.NewApplication(),
		messagesChan: make(chan types.DebateMessage, 100),
		commandsChan: make(chan string, 10),
		agent:        agent,
		maxMessages:  100,
		messages:     []types.DebateMessage{},
		lastDrawTime: time.Now(),
		drawDebounce: 100 * time.Millisecond, // Prevent too frequent redraws
	}

	ui.setupUI()
	return ui
}

// setupUI sets up the terminal UI
func (ui *UI) setupUI() {
	// Create the messages view
	ui.messagesView = tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() {
			// Debounce draws to prevent UI flickering
			if time.Since(ui.lastDrawTime) > ui.drawDebounce {
				ui.app.Draw()
				ui.lastDrawTime = time.Now()
			}
		})
	ui.messagesView.SetBorder(true).SetTitle(" Debate ")
	ui.messagesView.SetScrollable(true)
	ui.messagesView.SetWordWrap(true)

	// Create the input field
	ui.inputField = tview.NewInputField().
		SetLabel("> ").
		SetFieldWidth(0).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				command := ui.inputField.GetText()
				if command != "" {
					ui.commandsChan <- command
					ui.inputField.SetText("")
				}
			}
		})
	ui.inputField.SetBorder(true).SetTitle(" Command (style, personality, timeout, new) ")

	// Create the main layout with just debate area and input field
	mainFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(ui.messagesView, 0, 1, false).
		AddItem(ui.inputField, 3, 0, true)

	// Set the root and focus
	ui.app.SetRoot(mainFlex, true)
	ui.app.SetFocus(ui.inputField)

	// Add key handlers for better navigation
	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyPgUp:
			// Page up in messages view
			row, _ := ui.messagesView.GetScrollOffset()
			ui.messagesView.ScrollTo(row-5, 0)
			return nil
		case tcell.KeyPgDn:
			// Page down in messages view
			row, _ := ui.messagesView.GetScrollOffset()
			ui.messagesView.ScrollTo(row+5, 0)
			return nil
		case tcell.KeyHome:
			// Scroll to top
			ui.messagesView.ScrollToBeginning()
			return nil
		case tcell.KeyEnd:
			// Scroll to bottom
			ui.messagesView.ScrollToEnd()
			return nil
		case tcell.KeyTab:
			// Handle focus cycling
			if ui.app.GetFocus() == ui.inputField {
				ui.app.SetFocus(ui.messagesView)
			} else {
				ui.app.SetFocus(ui.inputField)
			}
			return nil
		}
		return event
	})
}

// Start starts the UI
func (ui *UI) Start(ctx context.Context) error {
	// Start the agent
	if err := ui.agent.Start(ctx, ui.messagesChan, ui.commandsChan); err != nil {
		return err
	}

	// Process messages
	go ui.processMessages(ctx)

	// Handle application exit
	go func() {
		<-ctx.Done()
		ui.app.Stop()
	}()

	// Run the application
	if err := ui.app.Run(); err != nil {
		log.Printf("UI error: %v", err)
		return err
	}

	return nil
}

// Stop stops the UI
func (ui *UI) Stop() {
	ui.app.Stop()
}

// processMessages processes incoming messages
func (ui *UI) processMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ui.messagesChan:
			if !ok {
				return
			}
			ui.addMessage(msg)
		}
	}
}

// addMessage adds a message to the messages view
func (ui *UI) addMessage(msg types.DebateMessage) {
	ui.messagesLock.Lock()
	defer ui.messagesLock.Unlock()

	// Add the message to the list
	ui.messages = append(ui.messages, msg)

	// Trim the list if it's too long
	if len(ui.messages) > ui.maxMessages {
		ui.messages = ui.messages[len(ui.messages)-ui.maxMessages:]
	}

	// Update the messages view
	ui.app.QueueUpdateDraw(func() {
		ui.messagesView.Clear()

		// Add a header for the debate topic if available
		if ui.agent.DebateTopic != "" {
			ui.messagesView.Write([]byte(fmt.Sprintf("[yellow]Current Topic:[white] %s\n\n", ui.agent.DebateTopic)))
		}

		// Add agent info at the top
		agentInfo := fmt.Sprintf("[yellow]Agent:[white] %s  [yellow]Style:[white] %s  [yellow]Topic:[white] %s\n\n",
			ui.agent.ID,
			string(ui.agent.Style),
			ui.agent.TopicName)
		ui.messagesView.Write([]byte(agentInfo))

		// Add all messages
		for _, m := range ui.messages {
			// Determine the color based on the sender
			var color string
			switch m.AgentID {
			case ui.agent.ID:
				color = "green"
			case "System":
				color = "yellow"
			default:
				color = "white"
			}

			// Format the message
			formattedMsg := fmt.Sprintf(
				"[blue]%s [%s]%s:[white] %s\n\n",
				m.Timestamp.Format("15:04:05"),
				color,
				m.AgentID,
				m.Content,
			)

			ui.messagesView.Write([]byte(formattedMsg))
		}

		// Scroll to the bottom
		ui.messagesView.ScrollToEnd()
	})
}

// FormatDebateStyle formats a debate style for display
func FormatDebateStyle(style types.DebateStyle) string {
	return strings.Title(string(style))
}
