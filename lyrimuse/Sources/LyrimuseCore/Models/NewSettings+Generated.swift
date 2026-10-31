// 由 scripts/gen-feature-list.py 按 shared/feature-list.json 的 new_settings 生成,别手改。

/// 每一版新加的设置:设置搜索目录里的 titleKey,按版本从旧到新。App 在这些设置的标题后面挂小蓝点(`NewSettingsMarks`)。
public enum NewSettingsCatalog {
    public static let releases: [(version: String, titleKeys: [String])] = [
        ("1.12.0", [
            "歌词来源",
            "优先用歌曲文件自带的歌词",
            "标注读音的语言",
            "日语读音",
            "逐字歌词格式",
            "歌名卡",
            "广告时隐藏",
            "进度线",
            "屏幕",
            "左耳",
            "右耳",
            "多行歌词",
            "广告时收起",
            "触控板手势",
            "歌词位置",
            "Spotify Canvas",
            "临时展开灵动岛",
            "图标换成专辑封面",
            "图标大小",
            "代理",
            "缓存",
            "发送匿名使用数据",
            "听歌报告",
        ]),
    ]
}
