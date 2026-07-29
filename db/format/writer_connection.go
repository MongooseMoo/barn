package format

import "fmt"

// SetActiveConnections sets the player/listener pairs saved at checkpoint.
func (w *Writer) SetActiveConnections(connections []ActiveConnection) {
	w.activeConnections = append([]ActiveConnection(nil), connections...)
}

func (w *Writer) writeActiveConnections() error {
	if err := w.writeString(fmt.Sprintf("%d active connections with listeners", len(w.activeConnections))); err != nil {
		return err
	}
	for _, connection := range w.activeConnections {
		if _, err := fmt.Fprintf(w.w, "%d %d\n", connection.Player, connection.Listener); err != nil {
			return err
		}
	}
	return nil
}
