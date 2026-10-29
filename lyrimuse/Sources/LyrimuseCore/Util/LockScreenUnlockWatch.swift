/// 锁屏那份灵动岛露着时轮询会话的锁屏状态,比解锁通知先一步发现已经解锁(App 侧 `NotchLockScreenPresenter`)。
/// 锁屏通知到时会话字典不一定已经标上锁着:先见到一次锁着,之后见到没锁才算解锁。
public struct LockScreenUnlockWatch: Equatable, Sendable {
    private var sawLocked = false

    public init() {}

    /// 记下这一次读到的会话状态,返回是不是该按解锁处理。
    public mutating func observe(sessionLocked: Bool) -> Bool {
        if sessionLocked {
            sawLocked = true
            return false
        }
        return sawLocked
    }
}
