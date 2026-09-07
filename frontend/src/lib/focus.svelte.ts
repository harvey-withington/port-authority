// "Show me that device": an insight asks for a device to be scrolled into
// view and flashed. The token increments per request so repeating the
// same id still flashes.

interface FocusRequest {
  id: string | null
  token: number
}

let request = $state<FocusRequest>({ id: null, token: 0 })

export const focus = {
  get id(): string | null {
    return request.id
  },
  get token(): number {
    return request.token
  },
  request(id: string): void {
    request = { id, token: request.token + 1 }
  },
}
