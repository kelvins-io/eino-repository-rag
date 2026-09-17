import { onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'

function getSpeechRecognitionCtor() {
  if (typeof window === 'undefined') return null
  return window.SpeechRecognition || window.webkitSpeechRecognition || null
}

export function useSpeechInput(query) {
  const listening = ref(false)
  const recognizedText = ref('')
  const supported = ref(!!getSpeechRecognitionCtor())

  let recognition = null
  let shouldRestart = false
  let finalTranscript = ''
  let commitTimer = null

  function clearCommitTimer() {
    if (commitTimer) {
      clearTimeout(commitTimer)
      commitTimer = null
    }
  }

  function commitToInput() {
    clearCommitTimer()
    const text = recognizedText.value.trim()
    recognizedText.value = ''
    finalTranscript = ''
    if (!text) {
      ElMessage.info('未识别到语音内容')
      return
    }
    const current = (query.value || '').trim()
    query.value = current ? `${current} ${text}` : text
  }

  function ensureRecognition() {
    if (recognition) return recognition
    const Ctor = getSpeechRecognitionCtor()
    if (!Ctor) return null

    recognition = new Ctor()
    recognition.lang = 'zh-CN'
    recognition.continuous = true
    recognition.interimResults = true

    recognition.onresult = (event) => {
      let interim = ''
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const piece = event.results[i][0]?.transcript || ''
        if (event.results[i].isFinal) {
          finalTranscript += piece
        } else {
          interim += piece
        }
      }
      recognizedText.value = `${finalTranscript}${interim}`.trim()
    }

    recognition.onerror = (event) => {
      if (event.error === 'aborted' || event.error === 'no-speech') return
      shouldRestart = false
      listening.value = false
      if (event.error === 'not-allowed' || event.error === 'service-not-allowed') {
        ElMessage.error('无法使用麦克风，请检查浏览器权限')
        return
      }
      if (event.error === 'network') {
        ElMessage.error('语音识别服务网络异常，请稍后重试')
        return
      }
      ElMessage.error('语音识别失败，请稍后重试')
    }

    recognition.onend = () => {
      if (shouldRestart && listening.value) {
        if (finalTranscript && !/[\s，。！？,.!?]$/.test(finalTranscript)) {
          finalTranscript += ' '
        }
        try {
          recognition.start()
          return
        } catch {
          // fall through and commit captured text
        }
      }
      if (listening.value) {
        listening.value = false
        // 部分浏览器会在 stop 后才给出最后一段识别结果
        clearCommitTimer()
        commitTimer = setTimeout(commitToInput, 80)
      }
    }

    return recognition
  }

  function start() {
    const rec = ensureRecognition()
    if (!rec) {
      ElMessage.warning('当前浏览器不支持语音识别，请使用 Chrome 或 Edge')
      return
    }
    clearCommitTimer()
    finalTranscript = ''
    recognizedText.value = ''
    shouldRestart = true
    listening.value = true
    try {
      rec.start()
    } catch (err) {
      if (err?.name === 'InvalidStateError') return
      shouldRestart = false
      listening.value = false
      ElMessage.error('无法开始语音识别')
    }
  }

  function stop() {
    if (!listening.value) return
    shouldRestart = false
    try {
      recognition.stop()
    } catch {
      listening.value = false
      commitToInput()
    }
  }

  function toggle() {
    if (listening.value) stop()
    else start()
  }

  onUnmounted(() => {
    shouldRestart = false
    listening.value = false
    clearCommitTimer()
    try {
      recognition?.abort()
    } catch {
      // ignore
    }
  })

  return {
    listening,
    recognizedText,
    supported,
    toggle,
    start,
    stop,
  }
}
