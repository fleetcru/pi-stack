# kotlinx.serialization — keep serializable classes and their companion serializers
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.AnnotationsKt

-keepclassmembers @kotlinx.serialization.Serializable class ** {
    *** Companion;
}
-keepclasseswithmembers class **$$serializer {
    *** INSTANCE;
}

# Keep all @Serializable model classes used by the API client
-keep class com.example.picompanion.data.model.** { *; }
-keep class com.example.picompanion.data.settings.** { *; }
-keep class com.example.picompanion.data.api.PromptImage { *; }

# OkHttp — suppress warnings from the platform
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**

# Coroutines
-keepnames class kotlinx.coroutines.internal.MainDispatcherFactory {}
-keepnames class kotlinx.coroutines.CoroutineExceptionHandler {}
