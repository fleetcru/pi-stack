import { useCallback, useRef, useState } from "react"

export interface PendingImage {
  id: string
  file: File
  previewUrl: string
  base64Data?: string
  mimeType: string
}

export interface ImageContent {
  type: "image"
  data: string
  mimeType: string
}

const MAX_IMAGE_SIZE = 20 * 1024 * 1024 // 20 MB
const MAX_IMAGES = 10

let nextId = 0

function isImageFile(file: File): boolean {
  return file.type.startsWith("image/")
}

function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result
      if (typeof result === "string") {
        // Strip the data URL prefix (e.g. "data:image/png;base64,")
        const comma = result.indexOf(",")
        resolve(comma >= 0 ? result.slice(comma + 1) : result)
      } else {
        reject(new Error("Unexpected FileReader result type"))
      }
    }
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

export function useImageAttachments() {
  const [images, setImages] = useState<PendingImage[]>([])
  const [error, setError] = useState<string | undefined>()
  const inputRef = useRef<HTMLInputElement>(null)

  const addFiles = useCallback(async (files: File[]) => {
    setError(undefined)
    const imageFiles = files.filter(isImageFile)
    if (imageFiles.length === 0) return

    setImages((current) => {
      const remaining = MAX_IMAGES - current.length
      if (remaining <= 0) {
        setError(`Maximum ${MAX_IMAGES} images allowed`)
        return current
      }
      const toAdd = imageFiles.slice(0, remaining)
      if (imageFiles.length > remaining) {
        setError(`Only ${MAX_IMAGES} images allowed (dropped ${imageFiles.length})`)
      }

      const newImages: PendingImage[] = toAdd
        .filter((file) => {
          if (file.size > MAX_IMAGE_SIZE) {
            setError(`"${file.name}" exceeds 20 MB limit`)
            return false
          }
          return true
        })
        .map((file) => ({
          id: `img-${++nextId}`,
          file,
          previewUrl: URL.createObjectURL(file),
          mimeType: file.type || "image/png",
        }))

      return [...current, ...newImages]
    })
  }, [])

  const removeImage = useCallback((id: string) => {
    setImages((current) => {
      const target = current.find((img) => img.id === id)
      if (target) URL.revokeObjectURL(target.previewUrl)
      return current.filter((img) => img.id !== id)
    })
  }, [])

  const clearImages = useCallback(() => {
    setImages((current) => {
      for (const img of current) URL.revokeObjectURL(img.previewUrl)
      return []
    })
    setError(undefined)
  }, [])

  /** Convert all pending images to base64 ImageContent for the API. */
  const toImageContent = useCallback(async (): Promise<ImageContent[]> => {
    if (images.length === 0) return []

    const results = await Promise.all(
      images.map(async (img) => {
        const base64 = img.base64Data ?? (await readFileAsBase64(img.file))
        return {
          type: "image" as const,
          data: base64,
          mimeType: img.mimeType,
        }
      })
    )
    return results
  }, [images])

  /** Handle paste events — extract images from clipboard. */
  const handlePaste = useCallback(
    (event: React.ClipboardEvent) => {
      const items = Array.from(event.clipboardData?.items ?? [])
      const imageFiles: File[] = []
      for (const item of items) {
        if (item.type.startsWith("image/")) {
          const file = item.getAsFile()
          if (file) imageFiles.push(file)
        }
      }
      if (imageFiles.length > 0) {
        event.preventDefault()
        void addFiles(imageFiles)
      }
    },
    [addFiles]
  )

  /** Handle drag over — indicate copy drop. */
  const handleDragOver = useCallback((event: React.DragEvent) => {
    if (event.dataTransfer.types.includes("Files")) {
      event.preventDefault()
      event.dataTransfer.dropEffect = "copy"
    }
  }, [])

  /** Handle drop — extract image files. */
  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      const files = Array.from(event.dataTransfer?.files ?? [])
      const imageFiles = files.filter(isImageFile)
      if (imageFiles.length > 0) {
        event.preventDefault()
        void addFiles(imageFiles)
      }
    },
    [addFiles]
  )

  /** Open the native file picker. */
  const openPicker = useCallback(() => {
    inputRef.current?.click()
  }, [])

  const handlePickerChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(event.target.files ?? [])
      if (files.length > 0) void addFiles(files)
      // Reset so the same file can be picked again
      event.target.value = ""
    },
    [addFiles]
  )

  return {
    images,
    error,
    inputRef,
    addFiles,
    removeImage,
    clearImages,
    toImageContent,
    handlePaste,
    handleDragOver,
    handleDrop,
    openPicker,
    handlePickerChange,
  }
}
