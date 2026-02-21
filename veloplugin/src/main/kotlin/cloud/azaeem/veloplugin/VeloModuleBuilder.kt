package cloud.azaeem.veloplugin

import com.fasterxml.jackson.databind.DeserializationFeature
import com.fasterxml.jackson.module.kotlin.jacksonObjectMapper
import com.fasterxml.jackson.module.kotlin.readValue
import com.intellij.ide.util.PropertiesComponent
import com.intellij.ide.util.projectWizard.ModuleBuilder
import com.intellij.ide.util.projectWizard.ModuleWizardStep
import com.intellij.ide.util.projectWizard.WizardContext
import com.intellij.openapi.Disposable
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.application.ModalityState
import com.intellij.openapi.fileChooser.FileChooser
import com.intellij.openapi.fileChooser.FileChooserDescriptorFactory
import com.intellij.openapi.module.ModuleType
import com.intellij.openapi.module.ModuleTypeManager
import com.intellij.openapi.options.ConfigurationException
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VfsUtil
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ide.projectView.ProjectView
import com.intellij.openapi.roots.ModifiableRootModel
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.openapi.ui.Messages
import com.intellij.openapi.util.IconLoader
import com.intellij.ui.components.JBCheckBox
import com.intellij.ui.components.JBTextField
import java.awt.BorderLayout
import java.awt.Dimension
import java.awt.GridBagConstraints
import java.awt.GridBagLayout
import java.awt.Insets
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.charset.StandardCharsets
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import java.nio.file.attribute.FileTime
import java.time.Duration
import java.time.Instant
import java.util.Base64
import java.util.concurrent.TimeUnit
import java.util.zip.ZipEntry
import java.util.zip.ZipInputStream
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.IvParameterSpec
import javax.crypto.spec.SecretKeySpec
import javax.swing.AbstractButton
import javax.swing.BorderFactory
import javax.swing.Box
import javax.swing.BoxLayout
import javax.swing.ButtonGroup
import javax.swing.DefaultComboBoxModel
import javax.swing.Icon
import javax.swing.JComboBox
import javax.swing.JCheckBox
import javax.swing.JComponent
import javax.swing.JLabel
import javax.swing.JPanel
import javax.swing.JRadioButton
import javax.swing.JScrollPane
import javax.swing.SwingUtilities

class VeloModuleBuilder : ModuleBuilder() {
    private var projectName: String = "my_app"
    private var orgName: String = "com.company"
    private var platforms: MutableSet<String> = mutableSetOf("android", "ios")
    private var apiUrl: String = ""

    override fun getBuilderId() = "cloud.azaeem.veloplugin.VeloModuleBuilder"
    override fun getPresentableName() = "Velo Flutter Project"
    override fun getDescription() = "Create a new Flutter project using the Velo backend"
    override fun getWeight() = 2000
    override fun getNodeIcon(): Icon = IconLoader.getIcon("/icons/velo.svg", VeloModuleBuilder::class.java)

    override fun setupRootModel(modifiableRootModel: ModifiableRootModel) {
        val contentEntry = doAddContentEntry(modifiableRootModel)
        val outputDir = contentEntry?.file?.path ?: return
        val catalog = try {
            loadCatalog()
        } catch (e: Exception) {
            val msg = e.message ?: "Unknown error from backend"
            throw ConfigurationException("Failed to load Velo catalog: $msg")
        }

        val selectionDialog = VeloSelectionDialog(modifiableRootModel.project, catalog)
        if (!selectionDialog.showAndGet()) {
            return
        }

        val selectedBlocks = selectionDialog.getSelectedBlocks()
        val selectedTemplateId = selectionDialog.getSelectedTemplateId()

        try {
            generateProjectWithBackend(
                project = modifiableRootModel.project,
                outputDir = outputDir,
                catalog = catalog,
                selectedBlockIds = selectedBlocks,
                selectedTemplateId = selectedTemplateId
            )
        } catch (e: Exception) {
            throw ConfigurationException("Failed to create project with Velo backend: ${e.message}")
        }
    }

    class NewPatternAction : AnAction("New Pattern", "Create a new feature pattern from Velo", null) {
        override fun update(e: AnActionEvent) {
            val vFile = e.getData(CommonDataKeys.VIRTUAL_FILE)
            e.presentation.isEnabledAndVisible = vFile != null && vFile.isDirectory
        }

        override fun actionPerformed(e: AnActionEvent) {
            val project = e.project ?: return
            val vFile = e.getData(CommonDataKeys.VIRTUAL_FILE) ?: return
            if (!vFile.isDirectory) return

            val builder = VeloModuleBuilder()
            val catalog = try {
                builder.loadCatalog()
            } catch (ex: Exception) {
                val msg = ex.message ?: "Unknown error from backend"
                Messages.showErrorDialog(project, "Failed to load patterns from backend: $msg", "Velo Patterns")
                return
            }

            if (catalog.patterns.isEmpty()) {
                Messages.showInfoMessage(project, "No patterns configured in the Velo admin dashboard.", "Velo Patterns")
                return
            }

            val options = catalog.patterns.map { it.label.ifBlank { it.id } }.toTypedArray()
            val initial = options.firstOrNull()
            val selectedLabel = Messages.showEditableChooseDialog(
                "Select a pattern to apply into this directory.",
                "New Pattern",
                null,
                options,
                initial,
                null
            ) ?: return

            val pattern = catalog.patterns.firstOrNull { it.label == selectedLabel || it.id == selectedLabel } ?: return

            val featureName = Messages.showInputDialog(
                project,
                "Enter feature name. This replaces the pattern placeholder.",
                "Feature Name",
                null
            )?.trim().orEmpty()
            if (featureName.isEmpty()) return

            val api = builder.resolveApiBaseUrl()
            val key = builder.loadEncryptionKey()
            val cacheDir = builder.resolveCacheDir()
            builder.purgeOldCache(cacheDir, Duration.ofDays(30))

            val encrypted = try {
                builder.getEncryptedPatternBlob(api, cacheDir, pattern.id)
            } catch (ex: Exception) {
                val msg = ex.message ?: "Unknown error"
                Messages.showErrorDialog(project, "Failed to download pattern: $msg", "Velo Patterns")
                return
            }

            val plain = try {
                builder.decryptPayload(key, encrypted)
            } catch (ex: Exception) {
                val msg = ex.message ?: "Unknown error"
                Messages.showErrorDialog(project, "Failed to decrypt pattern payload: $msg", "Velo Patterns")
                return
            }

            builder.applyPatternToDirectory(project, vFile, pattern, featureName, plain)
        }
    }

    override fun getModuleType(): ModuleType<*> {
        return ModuleTypeManager.getInstance().findByID(VeloModuleType.ID) ?: VeloModuleType.getInstance()
    }

    override fun getCustomOptionsStep(context: WizardContext, parentDisposable: Disposable): ModuleWizardStep? {
        return VeloWizardStep(this, context)
    }

    internal fun resolveApiBaseUrl(): String {
        val envApi = System.getenv("VELOCLI_API_URL")?.trim().orEmpty()
        if (envApi.isNotEmpty()) {
            return envApi
        }
        return "http://localhost:9999"
    }

    internal fun loadCatalog(): Catalog {
        val api = resolveApiBaseUrl()
        val envCatalog = System.getenv("VELOCLI_CATALOG_URL")?.trim().orEmpty()
        val url = if (envCatalog.isNotEmpty()) envCatalog else api.trimEnd('/') + "/api/v1/catalog"
        val client = HttpClient.newBuilder().build()
        val request = HttpRequest.newBuilder()
            .uri(URI(url))
            .GET()
            .header("Accept", "application/json")
            .build()
        val response = try {
            client.send(request, HttpResponse.BodyHandlers.ofString())
        } catch (e: Exception) {
            val reason = e.message ?: e.javaClass.simpleName
            throw IllegalStateException("Backend catalog request to $url failed: $reason")
        }
        val body = response.body()
        if (response.statusCode() != 200) {
            throw IllegalStateException("Backend catalog request to $url failed with HTTP ${response.statusCode()}")
        }
        val mapper = jacksonObjectMapper().configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false)
        return try {
            mapper.readValue(body)
        } catch (_: Exception) {
            throw IllegalStateException("Backend catalog response was not valid JSON")
        }
    }

    internal fun generateProjectWithBackend(
        project: Project?,
        outputDir: String,
        catalog: Catalog,
        selectedBlockIds: List<String>,
        selectedTemplateId: String?
    ) {
        val projectDir = Paths.get(outputDir)
        if (!Files.exists(projectDir)) {
            Files.createDirectories(projectDir)
        }

        val flutterCmds = resolveFlutterCommands()
        runFlutterCreate(project, flutterCmds, projectDir, projectName, orgName)

        val api = resolveApiBaseUrl()
        val blocksById = catalog.blocks.associateBy { it.id }
        val selectedBlocks = selectedBlockIds.mapNotNull { blocksById[it] }

        val selectedTemplate = selectedTemplateId?.let { id ->
            catalog.mainTemplates.firstOrNull { it.id == id }
        }

        val templateContent = selectedTemplate?.content.orEmpty()

        if (selectedBlocks.isEmpty() && (selectedTemplate == null || (selectedTemplate.blobId.isBlank() && templateContent.isBlank()))) {
            throw IllegalStateException("Select at least one block or a starting template")
        }

        val key = loadEncryptionKey()
        val cacheDir = resolveCacheDir()
        purgeOldCache(cacheDir, Duration.ofDays(30))

        applySelectedBlocks(
            apiBaseUrl = api,
            cacheDir = cacheDir,
            key = key,
            projectDir = projectDir,
            blocks = selectedBlocks,
            template = selectedTemplate
        )
    }

    class VeloWizardStep(private val builder: VeloModuleBuilder, private val context: WizardContext) : ModuleWizardStep() {
        private val panel = JPanel(GridBagLayout())
        private val orgField = JBTextField(builder.orgName)
        private val sdkField = JBTextField()
        private val androidCheck = JBCheckBox("Android", builder.platforms.contains("android"))
        private val iosCheck = JBCheckBox("iOS", builder.platforms.contains("ios"))
        private val webCheck = JBCheckBox("Web", builder.platforms.contains("web"))
        private val macosCheck = JBCheckBox("macOS", builder.platforms.contains("macos"))
        private val windowsCheck = JBCheckBox("Windows", builder.platforms.contains("windows"))
        private val linuxCheck = JBCheckBox("Linux", builder.platforms.contains("linux"))
        init {
            val storedSdk = PropertiesComponent.getInstance().getValue("velolabs.flutter.sdkDir")?.trim().orEmpty()
            if (storedSdk.isNotEmpty()) {
                sdkField.text = storedSdk
            }
        }

        override fun getComponent(): JComponent {
             val c = GridBagConstraints().apply {
                fill = GridBagConstraints.HORIZONTAL
                insets = Insets(6, 8, 6, 8)
                weightx = 1.0
                gridx = 0
                gridy = 0
            }
            
            addRow("Organization (com.example):", orgField, c)

            val sdkPanel = JPanel(BorderLayout())
            sdkPanel.add(sdkField, BorderLayout.CENTER)
            val browse = javax.swing.JButton("Browse")
            browse.addActionListener {
                val descriptor = FileChooserDescriptorFactory.createSingleFolderDescriptor()
                descriptor.title = "Select Flutter SDK Directory"
                val basePath = sdkField.text.trim().ifEmpty { builder.contentEntryPath ?: System.getProperty("user.home") }
                val initial = basePath?.let {
                    try {
                        com.intellij.openapi.vfs.VfsUtil.findFile(Paths.get(it), true)
                    } catch (_: Exception) {
                        null
                    }
                }
                val folder = FileChooser.chooseFile(descriptor, context.project, initial) ?: return@addActionListener
                sdkField.text = folder.path
            }
            sdkPanel.add(browse, BorderLayout.EAST)
            addRow("Flutter SDK:", sdkPanel, c)
            addRow("Platforms:", androidCheck, c)
            addRow("", iosCheck, c)
            addRow("", webCheck, c)
            addRow("", macosCheck, c)
            addRow("", windowsCheck, c)
            addRow("", linuxCheck, c)
            c.gridy++
            c.weighty = 1.0
            panel.add(JPanel(), c)
            
            return panel
        }
        
        private fun addRow(label: String, comp: JComponent, c: GridBagConstraints) {
            c.gridx = 0
            c.weightx = 0.0
            panel.add(JLabel(label), c)
            c.gridx = 1
            c.weightx = 1.0
            panel.add(comp, c)
            c.gridy++
        }

        override fun validate(): Boolean {
            val path = builder.contentEntryPath
            if (!path.isNullOrBlank()) {
                val dir = File(path)
                if (dir.exists()) {
                    Messages.showErrorDialog(
                        "A project already exists at this location. Please choose a different app name or directory.",
                        "Project Already Exists"
                    )
                    return false
                }
            }
            if (sdkField.text.trim().isEmpty()) {
                Messages.showErrorDialog(
                    "Select a Flutter SDK directory (folder that contains bin/flutter).",
                    "Flutter SDK Required"
                )
                return false
            }
            val selected = listOf(
                androidCheck.isSelected,
                iosCheck.isSelected,
                webCheck.isSelected,
                macosCheck.isSelected,
                windowsCheck.isSelected,
                linuxCheck.isSelected
            ).any { it }
            if (!selected) {
                Messages.showErrorDialog(
                    "Select at least one platform for your app.",
                    "No Platforms Selected"
                )
                return false
            }
            return true
        }

        override fun updateDataModel() {
            val nameFromWizard = builder.name?.trim().orEmpty()
            builder.projectName = if (nameFromWizard.isNotEmpty()) nameFromWizard else "my_app"
            builder.orgName = orgField.text.trim()
            val selected = mutableSetOf<String>()
            if (androidCheck.isSelected) selected.add("android")
            if (iosCheck.isSelected) selected.add("ios")
            if (webCheck.isSelected) selected.add("web")
            if (macosCheck.isSelected) selected.add("macos")
            if (windowsCheck.isSelected) selected.add("windows")
            if (linuxCheck.isSelected) selected.add("linux")
            if (selected.isEmpty()) {
                selected.add("android")
                selected.add("ios")
            }
            builder.platforms = selected
            PropertiesComponent.getInstance().setValue("velolabs.flutter.sdkDir", sdkField.text.trim())
        }
    }

    data class Catalog(
        val categories: List<CatalogCategory> = emptyList(),
        val blocks: List<CatalogBlock> = emptyList(),
        val mainTemplates: List<CatalogTemplate> = emptyList(),
        val patterns: List<CatalogPattern> = emptyList()
    )

    data class CatalogCategory(
        val id: String = "",
        val name: String = "",
        val selectionMode: String = ""
    )

    data class CatalogBlock(
        val id: String = "",
        val label: String = "",
        val categoryId: String = "",
        val description: String = "",
        val basePath: String = "",
        val conflicts: List<String> = emptyList(),
        val deps: Map<String, String> = emptyMap(),
        val mainTarget: String = "",
        val mainMode: String = "",
        val mainContent: String = "",
        val blobId: String = "",
        val updatedAt: String = ""
    )

    data class CatalogTemplate(
        val id: String = "",
        val label: String = "",
        val content: String = "",
        val blobId: String = "",
        val deps: Map<String, String> = emptyMap()
    )

    data class CatalogPattern(
        val id: String = "",
        val label: String = "",
        val description: String = "",
        val basePath: String = "",
        val placeholder: String = "",
        val blobId: String = "",
        val updatedAt: String = ""
    )

    private class TemplateItem(val id: String, private val display: String) {
        override fun toString(): String = display
    }

    private fun resolveFlutterCommands(): List<List<String>> {
        val commands = mutableListOf<List<String>>()

        val sdkDir = PropertiesComponent.getInstance().getValue("velolabs.flutter.sdkDir")?.trim().orEmpty()
        if (sdkDir.isNotEmpty()) {
            val flutterPath = Paths.get(sdkDir, "bin", "flutter").toString()
            commands += listOf(flutterPath)
        }

        val envBin = System.getenv("FLUTTER_BIN")?.trim().orEmpty()
        if (envBin.isNotEmpty()) {
            commands += listOf(envBin)
        }

        commands += listOf("flutter")
        commands += listOf("fvm", "flutter")
        return commands.distinct()
    }

    private fun runFlutterCreate(
        project: Project?,
        flutterCmds: List<List<String>>,
        projectDir: Path,
        projectName: String,
        orgName: String
    ) {
        var lastError: Throwable? = null
        for (base in flutterCmds) {
            try {
                val cmd = mutableListOf<String>().apply {
                    addAll(base)
                    add("create")
                    add(".")
                    add("--org")
                    add(orgName)
                    add("--project-name")
                    add(projectName)
                    if (platforms.isNotEmpty()) {
                        add("--platforms")
                        add(platforms.joinToString(","))
                    }
                }
                val process = ProcessBuilder(cmd)
                    .directory(projectDir.toFile())
                    .start()
                val finished = process.waitFor(180, TimeUnit.SECONDS)
                if (finished) {
                    val exitCode = process.exitValue()
                    if (exitCode == 0) {
                        return
                    }
                    val name = base.joinToString(" ")
                    lastError = IllegalStateException("$name create failed with exit code $exitCode")
                } else {
                    process.destroyForcibly()
                    val name = base.joinToString(" ")
                    lastError = IllegalStateException("$name create did not finish within 180 seconds")
                }
            } catch (e: Exception) {
                lastError = e
            }
        }
        if (project != null) {
            val answer = Messages.showYesNoDialog(
                project,
                "Velo could not find a working Flutter command.\n\n" +
                    "Click Yes to locate the Flutter SDK you want to use (the folder that contains bin/flutter).\n\n" +
                    "After saving the SDK, run the wizard again to create the project.",
                "Flutter SDK Not Found",
                "Yes",
                "No",
                null
            )
            if (answer == Messages.YES) {
                val descriptor = FileChooserDescriptorFactory.createSingleFolderDescriptor()
                descriptor.title = "Select Flutter SDK Directory"
                val chosen = FileChooser.chooseFile(descriptor, project, null)
                if (chosen != null) {
                    val sdkDir = chosen.path
                    PropertiesComponent.getInstance().setValue("velolabs.flutter.sdkDir", sdkDir)
                    throw IllegalStateException("Flutter SDK saved. Please run the wizard again.")
                }
            }
        }
        val tried = flutterCmds.joinToString(", ") { it.joinToString(" ") }
        throw IllegalStateException("Unable to run Flutter; tried: $tried; last error: ${lastError?.message}")
    }

    private fun resolveCacheDir(): Path {
        val custom = System.getenv("VELOCLI_CACHE_DIR")?.trim().orEmpty()
        if (custom.isNotEmpty()) {
            val p = Paths.get(custom)
            if (!Files.exists(p)) {
                Files.createDirectories(p)
            }
            return p
        }
        val userHome = System.getProperty("user.home")
        val base = Paths.get(userHome, ".cache", "velocli")
        if (!Files.exists(base)) {
            Files.createDirectories(base)
        }
        return base
    }

    private fun purgeOldCache(cacheDir: Path, ttl: Duration) {
        val cutoff = Instant.now().minus(ttl)
        Files.list(cacheDir).use { stream ->
            stream.forEach { path ->
                try {
                    val attrs = Files.getLastModifiedTime(path)
                    if (attrs.toInstant().isBefore(cutoff)) {
                        Files.deleteIfExists(path)
                    }
                } catch (_: Exception) {
                }
            }
        }
    }

    private fun getEncryptedPatternBlob(apiBaseUrl: String, cacheDir: Path, patternId: String): ByteArray {
        val cachePath = cacheDir.resolve("pattern_$patternId.bin")
        if (Files.isRegularFile(cachePath)) {
            Files.setLastModifiedTime(cachePath, FileTime.from(Instant.now()))
            return Files.readAllBytes(cachePath)
        }
        val client = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(30)).build()
        val url = apiBaseUrl.trimEnd('/') + "/api/v1/patterns/" + patternId + "/download"
        val request = HttpRequest.newBuilder()
            .uri(URI(url))
            .GET()
            .header("Accept", "application/octet-stream")
            .build()
        val response = client.send(request, HttpResponse.BodyHandlers.ofByteArray())
        if (response.statusCode() !in 200..299) {
            throw IllegalStateException("Pattern download failed with HTTP ${response.statusCode()}")
        }
        val body = response.body() ?: ByteArray(0)
        Files.createDirectories(cacheDir)
        val tmp = cachePath.resolveSibling(cachePath.fileName.toString() + ".tmp")
        Files.write(tmp, body)
        Files.move(tmp, cachePath)
        return body
    }

    private fun applyPatternToDirectory(
        project: Project,
        targetDir: VirtualFile,
        pattern: CatalogPattern,
        featureName: String,
        zipBytes: ByteArray
    ) {
        val basePath = if (pattern.basePath.isBlank()) "lib/features" else pattern.basePath
        val placeholder = if (pattern.placeholder.isBlank()) "{{FEATURE_NAME}}" else pattern.placeholder
        val targetBase = Paths.get(targetDir.path)

        val zipIn = ZipInputStream(ByteArrayInputStream(zipBytes))

        while (true) {
            val entry = zipIn.nextEntry ?: break
            val rawName = entry.name
            if (rawName.isBlank()) continue
            val sanitized = sanitizeZipPath(rawName)
            if (sanitized.isBlank()) continue

            val replacedRelative = sanitized.replace(placeholder, featureName)
            val relative = Paths.get(basePath).resolve(replacedRelative).normalize()
            val dest = targetBase.resolve(relative)

            if (entry.isDirectory) {
                Files.createDirectories(dest)
                continue
            }

            Files.createDirectories(dest.parent)
            val buffer = ByteArrayOutputStream()
            val buf = ByteArray(8192)
            while (true) {
                val read = zipIn.read(buf)
                if (read <= 0) break
                buffer.write(buf, 0, read)
            }
            var content = buffer.toByteArray()
            val text = runCatching { content.toString(Charsets.UTF_8) }.getOrNull()
            if (text != null) {
                val replaced = text.replace(placeholder, featureName)
                content = replaced.toByteArray(Charsets.UTF_8)
            }
            Files.write(dest, content)
        }

        VfsUtil.markDirtyAndRefresh(false, true, true, targetDir)

        ApplicationManager.getApplication().invokeLater({
            if (!project.isDisposed) {
                ProjectView.getInstance(project).refresh()
            }
        }, ModalityState.NON_MODAL)
    }

    private fun loadEncryptionKey(): ByteArray {
        val env = System.getenv("VELOCLI_DATA_KEY")?.trim().orEmpty()
        val candidates = mutableListOf<String>()
        if (env.isNotEmpty()) {
            candidates += env
        }
        val homeKeyPath = Paths.get(System.getProperty("user.home"), ".velocli", "data", ".key")
        if (Files.isRegularFile(homeKeyPath)) {
            runCatching { Files.readString(homeKeyPath).trim() }.onSuccess { text ->
                if (text.isNotEmpty()) candidates += text
            }
        }
        val repoKeyPath = Paths.get("/Volumes/TDKPS/tdkps/velocli/velocli-backend/data/.key")
        if (Files.isRegularFile(repoKeyPath)) {
            runCatching { Files.readString(repoKeyPath).trim() }.onSuccess { text ->
                if (text.isNotEmpty()) candidates += text
            }
        }
        for (raw in candidates) {
            val decoded = try {
                Base64.getDecoder().decode(raw)
            } catch (_: IllegalArgumentException) {
                continue
            }
            if (decoded.size == 32) {
                return decoded
            }
        }
        throw IllegalStateException("VELOCLI_DATA_KEY is not configured; set it in the environment or place the base64 key in ~/.velocli/data/.key")
    }

    private val headerV2 = "VELOENC2".toByteArray(StandardCharsets.UTF_8)
    private val nonceSize = 12

    private fun decryptPayload(key: ByteArray, data: ByteArray): ByteArray {
        if (data.size >= headerV2.size + nonceSize && data.copyOfRange(0, headerV2.size).contentEquals(headerV2)) {
            val nonce = data.copyOfRange(headerV2.size, headerV2.size + nonceSize)
            val ciphertext = data.copyOfRange(headerV2.size + nonceSize, data.size)
            val cipher = Cipher.getInstance("ChaCha20-Poly1305")
            val keySpec = SecretKeySpec(key, "ChaCha20")
            val ivSpec = IvParameterSpec(nonce)
            cipher.init(Cipher.DECRYPT_MODE, keySpec, ivSpec)
            return cipher.doFinal(ciphertext)
        }

        if (data.size < nonceSize) {
            throw IllegalStateException("ciphertext too short")
        }
        val nonce = data.copyOfRange(0, nonceSize)
        val ciphertext = data.copyOfRange(nonceSize, data.size)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        val keySpec = SecretKeySpec(key, "AES")
        val gcmSpec = GCMParameterSpec(128, nonce)
        cipher.init(Cipher.DECRYPT_MODE, keySpec, gcmSpec)
        return cipher.doFinal(ciphertext)
    }

    private fun applySelectedBlocks(
        apiBaseUrl: String,
        cacheDir: Path,
        key: ByteArray,
        projectDir: Path,
        blocks: List<CatalogBlock>,
        template: CatalogTemplate?
    ) {
        val effectiveTemplate = template

        if (effectiveTemplate != null && effectiveTemplate.blobId.isNotBlank()) {
            val enc = cacheGetOrFetchEncryptedTemplate(apiBaseUrl, cacheDir, effectiveTemplate.id)
            val plainZip = decryptPayload(key, enc)
            applyZipBlock(projectDir, CatalogBlock(basePath = "lib/"), plainZip)
            if (effectiveTemplate.deps.isNotEmpty()) {
                updatePubspecDeps(projectDir, effectiveTemplate.deps)
            }
        }

        val templateContent = effectiveTemplate?.content.orEmpty()
        if (blocks.isEmpty()) {
            if (templateContent.isNotEmpty()) {
                applyTemplateMain(projectDir, templateContent)
            }
            return
        }

        for (block in blocks) {
            val enc = cacheGetOrFetchEncrypted(apiBaseUrl, cacheDir, block.id)
            val plainZip = decryptPayload(key, enc)
            applyZipBlock(projectDir, block, plainZip)
            if (block.deps.isNotEmpty()) {
                updatePubspecDeps(projectDir, block.deps)
            }
            if (block.mainMode.isNotBlank() && block.mainMode.lowercase() != "none") {
                applyMainMutation(projectDir, block)
            }
        }

        if (templateContent.isNotEmpty()) {
            applyTemplateMain(projectDir, templateContent)
        }
    }

    private fun cacheGetOrFetchEncrypted(apiBaseUrl: String, cacheDir: Path, blockId: String): ByteArray {
        val cachePath = cacheDir.resolve("$blockId.bin")
        if (Files.isRegularFile(cachePath)) {
            try {
                val now = FileTime.from(Instant.now())
                Files.setLastModifiedTime(cachePath, now)
            } catch (_: Exception) {
            }
            return Files.readAllBytes(cachePath)
        }

        val client = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(30))
            .build()
        val url = apiBaseUrl.trimEnd('/') + "/api/v1/blocks/" + blockId + "/download"
        val request = HttpRequest.newBuilder()
            .uri(URI(url))
            .GET()
            .header("Accept", "application/octet-stream")
            .build()
        val response = client.send(request, HttpResponse.BodyHandlers.ofByteArray())
        if (response.statusCode() !in 200..299) {
            throw IllegalStateException("download failed: HTTP ${response.statusCode()}")
        }
        val body = response.body() ?: ByteArray(0)
        if (body.isEmpty()) {
            return body
        }
        if (!Files.exists(cacheDir)) {
            Files.createDirectories(cacheDir)
        }
        val tmp = cachePath.resolveSibling(cachePath.fileName.toString() + ".tmp")
        Files.write(tmp, body)
        Files.move(tmp, cachePath)
        return body
    }

    private fun cacheGetOrFetchEncryptedTemplate(apiBaseUrl: String, cacheDir: Path, templateId: String): ByteArray {
        val cachePath = cacheDir.resolve("tpl_$templateId.bin")
        if (Files.isRegularFile(cachePath)) {
            try {
                val now = FileTime.from(Instant.now())
                Files.setLastModifiedTime(cachePath, now)
            } catch (_: Exception) {
            }
            return Files.readAllBytes(cachePath)
        }

        val client = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(30))
            .build()
        val url = apiBaseUrl.trimEnd('/') + "/api/v1/templates/" + templateId + "/download"
        val request = HttpRequest.newBuilder()
            .uri(URI(url))
            .GET()
            .header("Accept", "application/octet-stream")
            .build()
        val response = client.send(request, HttpResponse.BodyHandlers.ofByteArray())
        if (response.statusCode() !in 200..299) {
            throw IllegalStateException("download failed: HTTP ${response.statusCode()}")
        }
        val body = response.body() ?: ByteArray(0)
        if (body.isEmpty()) {
            return body
        }
        if (!Files.exists(cacheDir)) {
            Files.createDirectories(cacheDir)
        }
        val tmp = cachePath.resolveSibling(cachePath.fileName.toString() + ".tmp")
        Files.write(tmp, body)
        Files.move(tmp, cachePath)
        return body
    }

    private fun sanitizeZipPath(p: String): String {
        var s = p.replace("\\", "/")
        s = s.trim()
        if (s.isEmpty()) return ""
        s = s.trimStart('/')
        val cleaned = java.nio.file.Paths.get(s).normalize().toString().replace("\\", "/")
        if (cleaned == "." || cleaned == ".." || cleaned.startsWith("../")) {
            return ""
        }
        return cleaned
    }

    private fun applyZipBlock(projectDir: Path, block: CatalogBlock, zipBytes: ByteArray) {
        if (zipBytes.isEmpty()) return
        val baseRaw = block.basePath.trim()
        var base = if (baseRaw.isEmpty()) "/" else baseRaw
        base = base.removePrefix("./")

        ZipInputStream(zipBytes.inputStream()).use { zis ->
            var entry: ZipEntry? = zis.nextEntry
            while (entry != null) {
                if (!entry.isDirectory) {
                    val zipPath = sanitizeZipPath(entry.name)
                    if (zipPath.isNotEmpty() && !zipPath.startsWith("__MACOSX/") && zipPath != "__MACOSX") {
                        val rel = if (base.startsWith("/")) {
                            val baseClean = base.removePrefix("/").trim()
                            if (baseClean.isEmpty()) {
                                zipPath
                            } else {
                                "$baseClean/$zipPath"
                            }
                        } else {
                            val baseClean = base.trim()
                            if (baseClean.isEmpty()) {
                                zipPath
                            } else {
                                "$baseClean/$zipPath"
                            }
                        }
                        val dst = projectDir.resolve(rel).normalize()
                        if (dst.startsWith(projectDir.normalize())) {
                            Files.createDirectories(dst.parent)
                            Files.newOutputStream(dst).use { out ->
                                val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                                while (true) {
                                    val read = zis.read(buffer)
                                    if (read <= 0) break
                                    out.write(buffer, 0, read)
                                }
                            }
                        }
                    }
                }
                zis.closeEntry()
                entry = zis.nextEntry
            }
        }
    }

    private fun updatePubspecDeps(projectDir: Path, deps: Map<String, String>) {
        if (deps.isEmpty()) return
        val pubspecPath = projectDir.resolve("pubspec.yaml")
        if (!Files.isRegularFile(pubspecPath)) return
        val text = Files.readString(pubspecPath)
        val yaml = org.yaml.snakeyaml.Yaml()
        val root = yaml.load<Any>(text) as? MutableMap<String, Any?> ?: mutableMapOf()
        val rawDeps = (root["dependencies"] as? MutableMap<String, Any?>) ?: mutableMapOf()
        for ((k, v) in deps) {
            val key = k.trim()
            val value = v.trim()
            if (key.isNotEmpty() && value.isNotEmpty()) {
                rawDeps[key] = value
            }
        }
        root["dependencies"] = rawDeps
        val updated = yaml.dump(root)
        Files.writeString(pubspecPath, updated)
    }

    private fun applyTemplateMain(projectDir: Path, content: String) {
        if (content.isBlank()) return
        val target = projectDir.resolve("lib").resolve("main.dart")
        if (!Files.exists(target.parent)) {
            Files.createDirectories(target.parent)
        }
        Files.writeString(target, content)
    }

    private fun applyMainMutation(projectDir: Path, block: CatalogBlock) {
        val mode = block.mainMode.trim().lowercase()
        if (mode.isEmpty() || mode == "none") return
        val targetRaw = block.mainTarget.ifBlank { "lib/main.dart" }
        var target = projectDir.resolve(targetRaw.removePrefix("/")).normalize()
        if (Files.isDirectory(target)) {
            target = target.resolve("main.dart")
        }
        val mainContent = block.mainContent
        when (mode) {
            "replace" -> {
                if (mainContent.isNotEmpty()) {
                    if (!Files.exists(target.parent)) {
                        Files.createDirectories(target.parent)
                    }
                    Files.writeString(target, mainContent)
                }
            }
            "prepend" -> {
                if (mainContent.isNotEmpty()) {
                    val existing = if (Files.isRegularFile(target)) Files.readString(target) else ""
                    var head = mainContent
                    if (head.isNotEmpty() && !head.endsWith("\n")) {
                        head += "\n"
                    }
                    val out = head + existing
                    Files.writeString(target, out)
                }
            }
            "append" -> {
                if (mainContent.isNotEmpty()) {
                    var existing = if (Files.isRegularFile(target)) Files.readString(target) else ""
                    if (existing.isNotEmpty() && !existing.endsWith("\n")) {
                        existing += "\n"
                    }
                    var out = existing + mainContent
                    if (!out.endsWith("\n")) {
                        out += "\n"
                    }
                    Files.writeString(target, out)
                }
            }
            "inject" -> {
                val snippet = mainContent.trim()
                if (snippet.isNotEmpty() && Files.isRegularFile(target)) {
                    val existing = Files.readString(target)
                    val injected = injectIntoDartMain(existing, snippet)
                    if (injected != null) {
                        Files.writeString(target, injected)
                    }
                }
            }
        }
    }

    private val dartMainRegex =
        Regex("""(?m)^\s*(?:Future<\s*void\s*>\s+|void\s+)?main\s*\([^)]*\)\s*(?:async\s*)?\{""")

    private fun injectIntoDartMain(src: String, snippetRaw: String): String? {
        val snippet = snippetRaw.trim()
        if (snippet.isEmpty()) return null
        if (src.contains(snippet)) return null
        val loc = dartMainRegex.find(src)?.range ?: return null
        val openBraceIndex = src.indexOf('{', loc.first)
        if (openBraceIndex < 0) return null
        val closeIdx = findMatchingBrace(src, openBraceIndex) ?: return null
        val body = src.substring(openBraceIndex + 1, closeIdx)
        if (body.contains(snippet)) {
            return null
        }
        val insertAt = body.indexOf("runApp(").takeIf { it >= 0 } ?: body.length
        val indent = detectIndentForInsert(body, insertAt)
        var ins = indentMultiline(snippet, indent)
        if (ins.isNotEmpty() && !ins.endsWith("\n")) {
            ins += "\n"
        }
        var newBody = body
        if (insertAt == body.length) {
            if (newBody.isNotBlank() && !newBody.endsWith("\n")) {
                newBody += "\n"
            }
            newBody += ins
        } else {
            if (insertAt > 0 && !newBody.substring(0, insertAt).endsWith("\n")) {
                ins = "\n$ins"
            }
            newBody = newBody.substring(0, insertAt) + ins + newBody.substring(insertAt)
        }
        return src.substring(0, openBraceIndex + 1) + newBody + src.substring(closeIdx)
    }

    private fun findMatchingBrace(src: String, openIdx: Int): Int? {
        if (openIdx < 0 || openIdx >= src.length || src[openIdx] != '{') {
            return null
        }
        var depth = 0
        for (i in openIdx until src.length) {
            when (src[i]) {
                '{' -> depth++
                '}' -> {
                    depth--
                    if (depth == 0) {
                        return i
                    }
                }
            }
        }
        return null
    }

    private fun detectIndentForInsert(body: String, at: Int): String {
        var idx = at.coerceIn(0, body.length)
        val lineStart = body.lastIndexOf('\n', idx.coerceAtLeast(0)).let { if (it < 0) 0 else it + 1 }
        var i = lineStart
        while (i < body.length && (body[i] == ' ' || body[i] == '\t')) {
            i++
        }
        val indent = body.substring(lineStart, i)
        return if (indent.isEmpty()) "  " else indent
    }

    private fun indentMultiline(s: String, indent: String): String {
        val lines = s.split('\n')
        return lines.joinToString("\n") { indent + it.trimEnd(' ', '\t') }
    }

    class NewVeloProjectAction : AnAction() {
        override fun actionPerformed(e: AnActionEvent) {
            val project = e.project ?: return
            val setupDialog = VeloProjectSetupDialog(project)
            if (!setupDialog.showAndGet()) {
                return
            }
            val projectName = setupDialog.getProjectName().trim()
            val org = setupDialog.getPackageId().trim()
            val baseDir = setupDialog.getBaseDir().trim()
            val sdkDir = setupDialog.getFlutterSdkDir().trim()
            if (projectName.isEmpty() || org.isEmpty() || baseDir.isEmpty()) {
                Messages.showErrorDialog(
                    project,
                    "Project name, package, and location are required.",
                    "VeloLabs"
                )
                return
            }
            if (sdkDir.isNotEmpty()) {
                PropertiesComponent.getInstance().setValue("velolabs.flutter.sdkDir", sdkDir)
            }
            val builder = VeloModuleBuilder()
            builder.projectName = projectName
            builder.orgName = org
            builder.platforms = mutableSetOf("android", "ios")
            val outputDir = Paths.get(baseDir, projectName).toString()
            val catalog = try {
                builder.loadCatalog()
            } catch (ex: Exception) {
                Messages.showErrorDialog(
                    project,
                    "VeloLabs could not load the catalog.\n\n${ex.message}",
                    "VeloLabs"
                )
                return
            }
            val selectionDialog = VeloSelectionDialog(project, catalog)
            if (!selectionDialog.showAndGet()) {
                return
            }
            val selectedBlocks = selectionDialog.getSelectedBlocks()
            val selectedTemplateId = selectionDialog.getSelectedTemplateId()
            com.intellij.openapi.progress.ProgressManager.getInstance().run(
                object : com.intellij.openapi.progress.Task.Backgroundable(project, "Creating Velo Flutter project", false) {
                    override fun run(indicator: com.intellij.openapi.progress.ProgressIndicator) {
                        try {
                            builder.generateProjectWithBackend(
                                project = project,
                                outputDir = outputDir,
                                catalog = catalog,
                                selectedBlockIds = selectedBlocks,
                                selectedTemplateId = selectedTemplateId
                            )
                            com.intellij.openapi.application.ApplicationManager.getApplication().invokeLater {
                                Messages.showInfoMessage(
                                    project,
                                    "Velo project created at:\n$outputDir",
                                    "VeloLabs"
                                )
                            }
                        } catch (ex: Exception) {
                            com.intellij.openapi.application.ApplicationManager.getApplication().invokeLater {
                                Messages.showErrorDialog(
                                    project,
                                    "VeloLabs could not create the project.\n\n${ex.message}",
                                    "VeloLabs"
                                )
                            }
                        }
                    }
                }
            )
        }
    }

    private class VeloProjectSetupDialog(private val project: Project) :
        DialogWrapper(project, true) {
        private val nameField = JBTextField("my_app")
        private val packageField = JBTextField("com.company")
        private val locationField = JBTextField()
        private val sdkField = JBTextField()

        init {
            title = "New Velo Flutter Project"
            val basePath = project.basePath
            if (basePath != null) {
                locationField.text = basePath
            }
            val storedSdk = PropertiesComponent.getInstance().getValue("velolabs.flutter.sdkDir")?.trim().orEmpty()
            if (storedSdk.isNotEmpty()) {
                sdkField.text = storedSdk
            }
            init()
        }

        override fun createCenterPanel(): JComponent {
            val panel = JPanel(GridBagLayout())
            val c = GridBagConstraints()
            c.insets = Insets(4, 4, 4, 4)
            c.fill = GridBagConstraints.HORIZONTAL
            c.weightx = 0.0
            c.gridy = 0

            fun addRow(labelText: String, field: JComponent, buttonText: String?, onClick: (() -> Unit)?) {
                c.gridx = 0
                c.weightx = 0.0
                panel.add(JLabel(labelText), c)
                c.gridx = 1
                c.weightx = 1.0
                panel.add(field, c)
                if (buttonText != null && onClick != null) {
                    c.gridx = 2
                    c.weightx = 0.0
                    val button = javax.swing.JButton(buttonText)
                    button.addActionListener { onClick() }
                    panel.add(button, c)
                }
                c.gridy++
            }

            addRow("Project name:", nameField, null, null)
            addRow("Package:", packageField, null, null)

            addRow(
                "Location:",
                locationField,
                "Browse"
            ) {
                val descriptor = FileChooserDescriptorFactory.createSingleFolderDescriptor()
                descriptor.title = "Select Project Location"
                val initial = com.intellij.openapi.vfs.VfsUtil.findFile(Paths.get(locationField.text.trim()), true)
                val folder = FileChooser.chooseFile(descriptor, project, initial) ?: return@addRow
                locationField.text = folder.path
            }

            addRow(
                "Flutter SDK:",
                sdkField,
                "Browse"
            ) {
                val descriptor = FileChooserDescriptorFactory.createSingleFolderDescriptor()
                descriptor.title = "Select Flutter SDK Directory"
                val initialPath = sdkField.text.trim().ifEmpty { locationField.text.trim() }
                val initial = if (initialPath.isNotEmpty()) {
                    com.intellij.openapi.vfs.VfsUtil.findFile(Paths.get(initialPath), true)
                } else {
                    null
                }
                val folder = FileChooser.chooseFile(descriptor, project, initial) ?: return@addRow
                sdkField.text = folder.path
            }

            panel.border = BorderFactory.createEmptyBorder(8, 8, 8, 8)
            return panel
        }

        fun getProjectName(): String = nameField.text
        fun getPackageId(): String = packageField.text
        fun getBaseDir(): String = locationField.text
        fun getFlutterSdkDir(): String = sdkField.text
    }

    private class VeloSelectionDialog(project: Project, private val catalog: Catalog) :
        DialogWrapper(project, true) {
        private val blockCheckboxes = linkedMapOf<AbstractButton, String>()
        private val templateBox = JComboBox<TemplateItem>()

        init {
            title = "VeloLabs · Features"
            init()
        }

        override fun createCenterPanel(): JComponent {
            val root = JPanel(BorderLayout())
            root.border = BorderFactory.createEmptyBorder(12, 12, 12, 12)
            root.preferredSize = Dimension(720, 520)

            val headerPanel = JPanel(BorderLayout())
            val titleLabel = JLabel("Choose what your app starts with")
            val titleFont = titleLabel.font
            titleLabel.font = titleFont.deriveFont(titleFont.style or java.awt.Font.BOLD, titleFont.size + 1f)
            headerPanel.add(titleLabel, BorderLayout.NORTH)

            val subtitle = JLabel("Pick VeloLabs building blocks on the left and an optional starting template on the right.")
            subtitle.border = BorderFactory.createEmptyBorder(4, 0, 8, 0)
            headerPanel.add(subtitle, BorderLayout.SOUTH)

            root.add(headerPanel, BorderLayout.NORTH)

            val mainPanel = JPanel(GridBagLayout())
            val gbc = GridBagConstraints().apply {
                fill = GridBagConstraints.BOTH
                weighty = 1.0
                insets = Insets(0, 0, 0, 0)
            }

            val blocksContent = JPanel()
            blocksContent.layout = BoxLayout(blocksContent, BoxLayout.Y_AXIS)

            for (category in catalog.categories) {
                val catLabel = JLabel(if (category.name.isNotBlank()) category.name else category.id)
                val catFont = catLabel.font
                catLabel.font = catFont.deriveFont(catFont.style or java.awt.Font.BOLD)
                catLabel.border = BorderFactory.createEmptyBorder(8, 0, 4, 0)
                blocksContent.add(catLabel)

                val blocksPanel = JPanel()
                blocksPanel.layout = BoxLayout(blocksPanel, BoxLayout.Y_AXIS)
                val blocksForCategory = catalog.blocks.filter { it.categoryId == category.id }
                val singleMode = category.selectionMode.equals("single", ignoreCase = true)
                if (singleMode) {
                    val group = ButtonGroup()
                    for (block in blocksForCategory) {
                        val text = if (block.label.isNotBlank()) block.label else block.id
                        val rb = JRadioButton(text)
                        group.add(rb)
                        blockCheckboxes[rb] = block.id
                        blocksPanel.add(rb)
                    }
                } else {
                    for (block in blocksForCategory) {
                        val text = if (block.label.isNotBlank()) block.label else block.id
                        val cb = JCheckBox(text)
                        blockCheckboxes[cb] = block.id
                        blocksPanel.add(cb)
                    }
                }
                if (blocksForCategory.isEmpty()) {
                    val emptyLabel = JLabel("No blocks in this category")
                    emptyLabel.border = BorderFactory.createEmptyBorder(0, 16, 0, 0)
                    blocksPanel.add(emptyLabel)
                }
                blocksPanel.border = BorderFactory.createEmptyBorder(0, 16, 0, 0)
                blocksContent.add(blocksPanel)
            }

            val blocksScroll = JScrollPane(blocksContent)
            blocksScroll.border = BorderFactory.createTitledBorder("Features")

            gbc.gridx = 0
            gbc.weightx = 0.6
            mainPanel.add(blocksScroll, gbc)

            val templatePanel = JPanel()
            templatePanel.layout = BoxLayout(templatePanel, BoxLayout.Y_AXIS)

            val templateLabel = JLabel("Starting template")
            val templateFont = templateLabel.font
            templateLabel.font = templateFont.deriveFont(templateFont.style or java.awt.Font.BOLD)
            templateLabel.border = BorderFactory.createEmptyBorder(0, 0, 4, 0)
            templatePanel.add(templateLabel)

            if (catalog.mainTemplates.isNotEmpty()) {
                val model = DefaultComboBoxModel<TemplateItem>()
                for (tpl in catalog.mainTemplates) {
                    val text = if (tpl.label.isNotBlank()) tpl.label else tpl.id
                    model.addElement(TemplateItem(tpl.id, text))
                }
                templateBox.model = model
                templateBox.maximumSize = Dimension(Int.MAX_VALUE, templateBox.preferredSize.height)
                if (model.size > 0) {
                    templateBox.selectedIndex = 0
                }
                templatePanel.add(templateBox)

                val helper = JLabel("Optional: choose a pre-wired main.dart layout.")
                helper.border = BorderFactory.createEmptyBorder(4, 0, 0, 0)
                templatePanel.add(helper)
            } else {
                val emptyTpl = JLabel("No templates available")
                emptyTpl.border = BorderFactory.createEmptyBorder(0, 16, 0, 0)
                templatePanel.add(emptyTpl)
            }

            val templateWrapper = JPanel(BorderLayout())
            templateWrapper.border = BorderFactory.createTitledBorder("Starting template")
            templateWrapper.add(templatePanel, BorderLayout.NORTH)

            gbc.gridx = 1
            gbc.weightx = 0.4
            gbc.insets = Insets(0, 12, 0, 0)
            mainPanel.add(templateWrapper, gbc)

            root.add(mainPanel, BorderLayout.CENTER)

            val footerPanel = JPanel(BorderLayout())
            val footer = JLabel("VeloLabs")
            footer.border = BorderFactory.createEmptyBorder(4, 0, 0, 0)
            val footerFont = footer.font
            footer.font = footerFont.deriveFont(footerFont.size - 1f)
            footerPanel.add(footer, BorderLayout.LINE_END)
            root.add(footerPanel, BorderLayout.SOUTH)
            return root
        }

        fun getSelectedBlocks(): List<String> {
            return blockCheckboxes.filter { it.key.isSelected }.values.toList()
        }

        fun getSelectedTemplateId(): String? {
            val item = templateBox.selectedItem as? TemplateItem ?: return null
            return item.id
        }
    }
}
