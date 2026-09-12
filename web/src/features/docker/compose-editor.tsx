import CodeMirror from '@uiw/react-codemirror'
import { yaml } from '@codemirror/lang-yaml'
import { useTheme } from '@/theme/use-theme'

export function ComposeEditor({
  value,
  onChange,
  readOnly = false,
}: {
  value: string
  onChange?: (value: string) => void
  readOnly?: boolean
}) {
  const { resolvedTheme } = useTheme()
  return (
    <div className="overflow-hidden rounded-lg border">
      <CodeMirror
        value={value}
        onChange={(next) => onChange?.(next)}
        extensions={[yaml()]}
        theme={resolvedTheme}
        height="340px"
        readOnly={readOnly}
        basicSetup={{
          lineNumbers: true,
          foldGutter: false,
          highlightActiveLine: !readOnly,
          autocompletion: false,
        }}
        aria-label="Compose editor"
      />
    </div>
  )
}
