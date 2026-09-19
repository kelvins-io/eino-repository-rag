import { onUnmounted, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'

const MAX_RECORD_MS = 60_000
const MIN_RECORD_MS = 400
const SILENCE_MS = 5_000
const SILENCE_RMS = 0.02
const SILENCE_POLL_MS = 100

function canRecord() {
  return (
    typeof window !== 'undefined' &&
    typeof MediaRecorder !== 'undefined' &&
    !!navigator.mediaDevices?.getUserMedia
  )
}

function pickMimeType() {
  const types = [
    'audio/webm;codecs=opus',
    'audio/webm',
    'audio/mp4',
    'audio/ogg;codecs=opus',
  ]
  if (typeof MediaRecorder === 'undefined' || typeof MediaRecorder.isTypeSupported !== 'function') {
    return ''
  }
  return types.find((type) => MediaRecorder.isTypeSupported(type)) || ''
}

function extFromMime(mime) {
  const type = (mime || '').toLowerCase()
  if (type.includes('mp4') || type.includes('m4a') || type.includes('aac')) return 'm4a'
  if (type.includes('ogg')) return 'ogg'
  if (type.includes('wav')) return 'wav'
  if (type.includes('mpeg') || type.includes('mp3')) return 'mp3'
  return 'webm'
}

function formatElapsed(ms) {
  const total = Math.max(0, Math.floor(ms / 1000))
  const mm = String(Math.floor(total / 60))
  const ss = String(total % 60).padStart(2, '0')
  return `${mm}:${ss}`
}

export function useSpeechInput(query, options = {}) {
  const listening = ref(false)
  const transcribing = ref(false)
  const statusText = ref('')
  const supported = ref(canRecord())

  let mediaStream = null
  let recorder = null
  let chunks = []
  let startedAt = 0
  let tickTimer = null
  let maxTimer = null
  let silenceTimer = null
  let audioContext = null
  let analyser = null
  let lastVoiceAt = 0
  let commitOnStop = true
  let stopping = false

  function clearTimers() {
    if (tickTimer) {
      clearInterval(tickTimer)
      tickTimer = null
    }
    if (maxTimer) {
      clearTimeout(maxTimer)
      maxTimer = null
    }
    if (silenceTimer) {
      clearInterval(silenceTimer)
      silenceTimer = null
    }
  }

  function stopAnalyser() {
    analyser = null
    if (audioContext) {
      const ctx = audioContext
      audioContext = null
      ctx.close().catch(() => {})
    }
  }

  function stopTracks() {
    stopAnalyser()
    mediaStream?.getTracks().forEach((track) => track.stop())
    mediaStream = null
  }

  function currentRms() {
    if (!analyser) return 0
    const data = new Uint8Array(analyser.fftSize)
    analyser.getByteTimeDomainData(data)
    let sum = 0
    for (let i = 0; i < data.length; i++) {
      const v = (data[i] - 128) / 128
      sum += v * v
    }
    return Math.sqrt(sum / data.length)
  }

  function startSilenceMonitor(stream) {
    const Ctx = window.AudioContext || window.webkitAudioContext
    if (!Ctx) return
    try {
      audioContext = new Ctx()
      if (audioContext.state === 'suspended') {
        audioContext.resume().catch(() => {})
      }
      const source = audioContext.createMediaStreamSource(stream)
      analyser = audioContext.createAnalyser()
      analyser.fftSize = 2048
      analyser.smoothingTimeConstant = 0.3
      source.connect(analyser)
    } catch {
      stopAnalyser()
      return
    }
    lastVoiceAt = Date.now()
    silenceTimer = setInterval(() => {
      if (!listening.value || stopping) return
      if (currentRms() >= SILENCE_RMS) {
        lastVoiceAt = Date.now()
        return
      }
      if (Date.now() - lastVoiceAt >= SILENCE_MS) {
        Message.info('检测到静音，开始识别')
        stop()
      }
    }, SILENCE_POLL_MS)
  }

  function commitText(text) {
    const value = (text || '').trim()
    if (!value) {
      Message.info('未识别到语音内容')
      return
    }
    const current = (query.value || '').trim()
    query.value = current ? `${current} ${value}` : value
  }

  async function transcribeBlob(file) {
    transcribing.value = true
    statusText.value = '正在识别，请稍候…'
    try {
      const prompt = options.getPrompt?.() || ''
      const data = await api.transcribeSpeech(file, { prompt })
      const text = typeof data === 'string' ? data : data?.text
      commitText(text)
      await options.onTranscribed?.()
    } catch {
      // axios interceptor already shows the error
    } finally {
      transcribing.value = false
      statusText.value = ''
    }
  }

  async function start() {
    if (!supported.value) {
      Message.warning('当前浏览器不支持录音，请使用 Chrome、Edge 或 Safari')
      return
    }
    if (listening.value || transcribing.value) return

    commitOnStop = true
    stopping = false
    chunks = []
    try {
      mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch {
      Message.error('无法使用麦克风，请检查浏览器权限')
      return
    }

    const mime = pickMimeType()
    try {
      recorder = mime
        ? new MediaRecorder(mediaStream, { mimeType: mime })
        : new MediaRecorder(mediaStream)
    } catch {
      stopTracks()
      Message.error('无法开始录音')
      return
    }

    recorder.ondataavailable = (event) => {
      if (event.data && event.data.size > 0) chunks.push(event.data)
    }
    recorder.onerror = () => {
      Message.error('录音失败，请稍后重试')
      commitOnStop = false
      stop()
    }
    recorder.onstop = async () => {
      stopping = false
      clearTimers()
      listening.value = false
      const mimeType = recorder?.mimeType || mime || 'audio/webm'
      recorder = null
      stopTracks()
      const elapsed = Date.now() - startedAt
      const shouldCommit = commitOnStop
      commitOnStop = true
      if (!shouldCommit) {
        chunks = []
        statusText.value = ''
        return
      }
      if (elapsed < MIN_RECORD_MS) {
        Message.info('录音时间过短，请重试')
        chunks = []
        statusText.value = ''
        return
      }
      const blob = new Blob(chunks, { type: mimeType.split(';')[0] || 'audio/webm' })
      chunks = []
      if (!blob.size) {
        Message.info('未采集到音频，请重试')
        statusText.value = ''
        return
      }
      const file = new File([blob], `speech.${extFromMime(mimeType)}`, {
        type: blob.type || 'audio/webm',
      })
      await transcribeBlob(file)
    }

    startedAt = Date.now()
    listening.value = true
    statusText.value = '正在录音 0:00'
    tickTimer = setInterval(() => {
      statusText.value = `正在录音 ${formatElapsed(Date.now() - startedAt)}`
    }, 250)
    maxTimer = setTimeout(() => {
      Message.info('已达到最长录音时长，开始识别')
      stop()
    }, MAX_RECORD_MS)

    startSilenceMonitor(mediaStream)

    try {
      recorder.start(200)
    } catch {
      clearTimers()
      listening.value = false
      recorder = null
      stopTracks()
      Message.error('无法开始录音')
    }
  }

  function stop(opts = {}) {
    if (opts.commit === false) commitOnStop = false
    if (!recorder && !listening.value) return
    if (stopping) return
    stopping = true
    clearTimers()
    if (recorder && recorder.state !== 'inactive') {
      try {
        recorder.stop()
        return
      } catch {
        // fall through
      }
    }
    listening.value = false
    recorder = null
    stopping = false
    stopTracks()
    if (!commitOnStop) statusText.value = ''
  }

  function toggle() {
    if (transcribing.value) return
    if (listening.value) stop()
    else start()
  }

  onUnmounted(() => {
    stop({ commit: false })
  })

  return {
    listening,
    transcribing,
    statusText,
    supported,
    toggle,
    start,
    stop,
  }
}
