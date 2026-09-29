import { useSyncExternalStore } from 'react'

const listeners = new Set<() => void>()
let currentTime = Date.now()
let timer: ReturnType<typeof setInterval> | undefined

function getSnapshot() {
  return currentTime
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  currentTime = Date.now()
  listener()

  if (!timer) {
    timer = setInterval(() => {
      currentTime = Date.now()
      listeners.forEach((notify) => notify())
    }, 60_000)
  }

  return () => {
    listeners.delete(listener)
    if (listeners.size === 0 && timer) {
      clearInterval(timer)
      timer = undefined
    }
  }
}

export function useCurrentTime() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}
