package cloud.azaeem.veloplugin

import com.intellij.openapi.module.ModuleType
import com.intellij.openapi.module.ModuleTypeManager
import com.intellij.openapi.util.IconLoader
import javax.swing.Icon

class VeloModuleType : ModuleType<VeloModuleBuilder>(ID) {

    companion object {
        const val ID: String = "VELO_MODULE_TYPE"

        fun getInstance(): VeloModuleType {
            return ModuleTypeManager.getInstance().findByID(ID) as VeloModuleType
        }
    }

    override fun createModuleBuilder(): VeloModuleBuilder {
        return VeloModuleBuilder()
    }

    override fun getName(): String {
        return "Velo Flutter Project"
    }

    override fun getDescription(): String {
        return "Create a new Flutter project from VeloLabs feature blocks and templates."
    }

    override fun getNodeIcon(isOpened: Boolean): Icon {
        return IconLoader.getIcon("/icons/velo.svg", VeloModuleType::class.java)
    }
}
