package cloud.azaeem.veloplugin

import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.execution.process.OSProcessHandler
import com.intellij.execution.process.ProcessEvent
import com.intellij.execution.process.ProcessListener
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileChooser.FileChooser
import com.intellij.openapi.fileChooser.FileChooserDescriptor
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.TextFieldWithBrowseButton
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.content.ContentFactory
import com.intellij.ui.components.JBPasswordField
import com.intellij.ui.components.JBScrollPane
import com.intellij.ui.components.JBTextArea
import com.intellij.ui.components.JBTextField
import java.awt.BorderLayout
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import java.awt.Insets
import java.nio.file.Files
import java.nio.file.Paths
import javax.swing.JButton
import javax.swing.JLabel
import javax.swing.JPanel
import javax.swing.SwingUtilities

class VeloToolWindowFactory : ToolWindowFactory {
    override fun shouldBeAvailable(project: Project) = true

    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        val panel = VeloPanel(project)
        val content = ContentFactory.getInstance().createContent(panel, "", false)
        toolWindow.contentManager.addContent(content)
    }
}

private class VeloPanel(private val project: Project) : JPanel(BorderLayout()) {
    private val velocliPath = TextFieldWithBrowseButton()
    private val apiUrl = JBTextField("")
    private val dataKey = JBPasswordField()
    private val outputDir = TextFieldWithBrowseButton()
    private val projectName = JBTextField("my_app")
    private val org = JBTextField("com.company")
    private val platforms = JBTextField("android,ios")
    private val blocks = JBTextField("")
    private val template = JBTextField("")
    private val console = JBTextArea()

    private var handler: OSProcessHandler? = null

    init {
        val form = JPanel(GridBagLayout())
        val c = GridBagConstraints().apply {
            fill = GridBagConstraints.HORIZONTAL
            insets = Insets(6, 8, 6, 8)
            weightx = 1.0
            gridx = 0
            gridy = 0
        }

        setupBrowseFields()
        preloadDefaults()

        addRow(form, c, "velocli path", velocliPath)
        addRow(form, c, "service url", apiUrl)
        addRow(form, c, "data key", dataKey)
        addRow(form, c, "output dir", outputDir)
        addRow(form, c, "project name", projectName)
        addRow(form, c, "org/package", org)
        addRow(form, c, "platforms", platforms)
        addRow(form, c, "block ids", blocks)
        addRow(form, c, "template id", template)

        val buttons = JPanel()
        val run = JButton("Generate")
        val stop = JButton("Stop")
        stop.isEnabled = false

        run.addActionListener {
            if (handler != null) return@addActionListener
            console.text = ""
            val cmd = buildCommand() ?: return@addActionListener
            val h = OSProcessHandler(cmd)
            handler = h
            run.isEnabled = false
            stop.isEnabled = true

            h.addProcessListener(object : ProcessListener {
                override fun onTextAvailable(event: ProcessEvent, outputType: com.intellij.openapi.util.Key<*>) {
                    val text = event.text
                    SwingUtilities.invokeLater {
                        console.append(text)
                        console.caretPosition = console.document.length
                    }
                }

                override fun processTerminated(event: ProcessEvent) {
                    SwingUtilities.invokeLater {
                        console.append("\n[exit code ${event.exitCode}]\n")
                        run.isEnabled = true
                        stop.isEnabled = false
                    }
                    handler = null
                }

                override fun startNotified(event: ProcessEvent) {}
                override fun processWillTerminate(event: ProcessEvent, willBeDestroyed: Boolean) {}
            })

            h.startNotify()
        }

        stop.addActionListener {
            handler?.destroyProcess()
        }

        buttons.add(run)
        buttons.add(stop)

        c.gridx = 0
        c.gridwidth = 2
        c.fill = GridBagConstraints.HORIZONTAL
        c.weightx = 1.0
        c.gridy++
        form.add(buttons, c)

        console.isEditable = false
        console.lineWrap = true
        console.wrapStyleWord = true

        add(form, BorderLayout.NORTH)
        add(JBScrollPane(console), BorderLayout.CENTER)
    }

    private fun setupBrowseFields() {
        velocliPath.addActionListener {
            val descriptor = FileChooserDescriptor(
                true,
                false,
                false,
                false,
                false,
                false
            )
            descriptor.title = "Select velocli binary"
            descriptor.description = "Select the velocli executable to run"
            val chosen = FileChooser.chooseFile(descriptor, project, null)
            if (chosen != null) {
                velocliPath.text = chosen.path
            }
        }

        outputDir.addActionListener {
            val descriptor = FileChooserDescriptor(
                false,
                true,
                false,
                false,
                false,
                false
            )
            descriptor.title = "Select output directory"
            descriptor.description = "Directory where generated projects will be created"
            val chosen = FileChooser.chooseFile(descriptor, project, null)
            if (chosen != null) {
                outputDir.text = chosen.path
            }
        }
    }

    private fun preloadDefaults() {
        val envBin = System.getenv("VELOCLI_BIN")?.trim().orEmpty()
        if (envBin.isNotEmpty()) {
            velocliPath.text = envBin
        } else {
            val base = project.basePath?.let { Paths.get(it) }
            if (base != null) {
                val candidate = base.resolve("velocli-cli").resolve("bin").resolve("velocli")
                if (Files.exists(candidate)) {
                    velocliPath.text = candidate.toString()
                }
            }
        }

        val envKey = System.getenv("VELOCLI_DATA_KEY")?.trim().orEmpty()
        if (envKey.isNotEmpty()) {
            dataKey.text = envKey
        } else {
            val base = project.basePath?.let { Paths.get(it) }
            if (base != null) {
                val keyPath = base.resolve("velocli-backend").resolve("data").resolve(".key")
                if (Files.isRegularFile(keyPath)) {
                    runCatching { Files.readString(keyPath).trim() }.onSuccess {
                        if (it.isNotEmpty()) dataKey.text = it
                    }
                }
            }
        }

        val base = project.basePath?.let { Paths.get(it) }
        if (base != null) {
            outputDir.text = base.resolve("output").toString()
        }
    }

    private fun buildCommand(): GeneralCommandLine? {
        val bin = velocliPath.text.trim()
        if (bin.isEmpty()) {
            appendLine("[error] velocli path is required (set VELOCLI_BIN or pick a file).")
            return null
        }
        val binPath = Paths.get(bin)
        if (!Files.exists(binPath)) {
            appendLine("[error] velocli binary not found: $bin")
            return null
        }

        val api = apiUrl.text.trim()
        val out = outputDir.text.trim()
        val name = projectName.text.trim()
        val orgText = org.text.trim()

        if (api.isEmpty() || out.isEmpty() || name.isEmpty() || orgText.isEmpty()) {
            appendLine("[error] api url, output dir, project name, and org are required.")
            return null
        }

        val args = mutableListOf(
            "create",
            "--api-url", api,
            "--output-dir", out,
            "--project-name", name,
            "--org", orgText,
            "--platforms", platforms.text.trim()
        )

        val blockText = blocks.text.trim()
        if (blockText.isNotEmpty()) {
            args += listOf("--blocks", blockText)
        }

        val tpl = template.text.trim()
        if (tpl.isNotEmpty()) {
            args += listOf("--template", tpl)
        }

        val cmd = GeneralCommandLine(binPath.toString())
            .withParameters(args)
            .withWorkDirectory(project.basePath)

        val key = String(dataKey.password).trim()
        if (key.isNotEmpty()) {
            cmd.environment["VELOCLI_DATA_KEY"] = key
        }

        return cmd
    }

    private fun appendLine(s: String) {
        console.append(s + "\n")
        console.caretPosition = console.document.length
    }
}

private fun addRow(form: JPanel, c: GridBagConstraints, label: String, field: java.awt.Component) {
    c.gridx = 0
    c.gridwidth = 1
    c.weightx = 0.0
    form.add(JLabel(label), c)

    c.gridx = 1
    c.weightx = 1.0
    form.add(field, c)

    c.gridy++
}

class VeloStartupActivity : com.intellij.openapi.startup.StartupActivity.DumbAware {
    override fun runActivity(project: Project) {
        ApplicationManager.getApplication().invokeLater {
            com.intellij.openapi.wm.ToolWindowManager.getInstance(project).getToolWindow("VeloCli")?.activate(null, true)
        }
    }
}
