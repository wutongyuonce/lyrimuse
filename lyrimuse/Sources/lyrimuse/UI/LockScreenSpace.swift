import AppKit

/// 锁屏之上的那一层:SkyLight 建一个绝对层级高于锁屏界面的 Space,窗口挪进去后锁屏时照样看得见。
///
/// 进了这个 Space 的窗口一直待在里面,要显示就 `orderFrontRegardless`、要藏就 `orderOut`;别关掉或销毁它
/// (挪进去的窗口被释放会让 SkyLight 崩)。Space 本身全进程一个,第一次用时建。
/// 私有函数运行时查找(同 `FullScreenSpaceMonitor`):查不到时 `adopt` 返回 false,调用方不显示。见 05 章决策 122。
@MainActor
enum LockScreenSpace {
    /// `kSLSSpaceAbsoluteLevelNotificationCenterAtScreenLock`:锁屏上通知中心那一层,高于锁屏界面(300)。
    private static let absoluteLevel: Int32 = 400

    private static var space: (connection: Int32, id: Int32)?

    /// 把窗口挪进锁屏之上的 Space。只挪一次,窗口之后一直留在里面。
    static func adopt(_ window: NSWindow) -> Bool {
        guard let functions = Self.functions, window.windowNumber > 0 else { return false }
        let target: (connection: Int32, id: Int32)
        if let space {
            target = space
        } else {
            let connection = functions.mainConnection()
            let id = functions.create(connection, 1, 0)
            guard id != 0 else { return false }
            _ = functions.setLevel(connection, id, absoluteLevel)
            functions.show(connection, [id] as CFArray)
            target = (connection, id)
            space = target
        }
        // 最后一个参数 7:从窗口原来所在的全部 Space 里拿掉。
        functions.addWindows(target.connection, target.id, [window.windowNumber] as CFArray, 7)
        return true
    }

    private typealias MainConnectionID = @convention(c) () -> Int32
    private typealias SpaceCreate = @convention(c) (Int32, Int32, Int32) -> Int32
    private typealias SpaceSetAbsoluteLevel = @convention(c) (Int32, Int32, Int32) -> Int32
    // 这两个没有返回值(按 Int32 接住的是寄存器里的残留,本机读到 205275136),别拿它判成败。
    private typealias ShowSpaces = @convention(c) (Int32, CFArray) -> Void
    private typealias SpaceAddWindowsAndRemoveFromSpaces = @convention(c) (Int32, Int32, CFArray, Int32) -> Void

    private static let functions: (mainConnection: MainConnectionID, create: SpaceCreate, setLevel: SpaceSetAbsoluteLevel,
                                   show: ShowSpaces, addWindows: SpaceAddWindowsAndRemoveFromSpaces)? = {
        guard let handle = dlopen("/System/Library/PrivateFrameworks/SkyLight.framework/Versions/A/SkyLight", RTLD_LAZY),
              let main = dlsym(handle, "SLSMainConnectionID"),
              let create = dlsym(handle, "SLSSpaceCreate"),
              let level = dlsym(handle, "SLSSpaceSetAbsoluteLevel"),
              let show = dlsym(handle, "SLSShowSpaces"),
              let add = dlsym(handle, "SLSSpaceAddWindowsAndRemoveFromSpaces") else { return nil }
        return (unsafeBitCast(main, to: MainConnectionID.self), unsafeBitCast(create, to: SpaceCreate.self),
                unsafeBitCast(level, to: SpaceSetAbsoluteLevel.self), unsafeBitCast(show, to: ShowSpaces.self),
                unsafeBitCast(add, to: SpaceAddWindowsAndRemoveFromSpaces.self))
    }()
}
