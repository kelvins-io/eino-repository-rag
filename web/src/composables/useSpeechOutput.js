import { onUnmounted, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'

export function useSpeechOutput() {
  const speakingIdx = ref(null)
  const loadingIdx = ref(null)

  let audio = null
  let objectUrl = ''
  let requestSeq = 0

  function releaseAudio() {
    if (audio) {
      audio.onended = null
      audio.onerror = null
      audio.pause()
      audio.src = ''
      audio = null
    }
    if (objectUrl) {
      URL.revokeObjectURL(objectUrl)
      objectUrl = ''
    }
  }

  function stop() {
    requestSeq += 1
    releaseAudio()
    speakingIdx.value = null
    loadingIdx.value = null
  }

  async function toggle(idx, text) {
    if (speakingIdx.value === idx || loadingIdx.value === idx) {
      stop()
      return
    }
    const value = (text || '').trim()
    if (!value || value === '(空回答)') {
      Message.info('没有可朗读的内容')
      return
    }

    stop()
    const seq = requestSeq
    loadingIdx.value = idx
    try {
      const blob = await api.synthesizeSpeech(value)
      if (seq !== requestSeq) return
      objectUrl = URL.createObjectURL(blob)
      audio = new Audio(objectUrl)
      audio.onended = () => {
        if (seq === requestSeq) stop()
      }
      audio.onerror = () => {
        if (seq === requestSeq) {
          Message.error('音频播放失败')
          stop()
        }
      }
      speakingIdx.value = idx
      loadingIdx.value = null
      await audio.play()
    } catch {
      if (seq === requestSeq) {
        loadingIdx.value = null
      }
    }
  }

  onUnmounted(stop)

  return {
    speakingIdx,
    loadingIdx,
    toggle,
    stop,
  }
}
