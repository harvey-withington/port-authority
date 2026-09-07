/**
 * starting   discovering the API (Wails Status() not ready yet)
 * connecting first fetch / WebSocket handshake in flight
 * live       WebSocket open, events arriving
 * polling    WebSocket down, refreshing /topology on a timer
 * offline    neither the socket nor polling can reach the service
 */
export type ConnectionState = 'starting' | 'connecting' | 'live' | 'polling' | 'offline'
