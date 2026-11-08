package sqlite

import (
	"fmt"
	"testing"
	"time"

	"echogallery/internal/storage"
)

func makeTimelinePhotos(t *testing.T, db *DB, count int) []*storage.Photo {
	t.Helper()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	photos := make([]*storage.Photo, 0, count)
	for i := 0; i < count; i++ {
		p := makePhoto(1, base.Add(time.Duration(i)*time.Hour))
		p.UUID = fmt.Sprintf("timeline-%02d", i)
		p.OriginalName = fmt.Sprintf("timeline-%02d.jpg", i)
		if err := db.SavePhoto(p); err != nil {
			t.Fatalf("保存测试图片失败: %v", err)
		}
		photos = append(photos, p)
	}
	return photos
}

func assertPhotoIDs(t *testing.T, photos []*storage.Photo, want []int64) {
	t.Helper()
	if len(photos) != len(want) {
		t.Fatalf("数量不符，得到 %d，期望 %d", len(photos), len(want))
	}
	for i, photo := range photos {
		if photo == nil || photo.ID != want[i] {
			var got int64
			if photo != nil {
				got = photo.ID
			}
			t.Fatalf("第 %d 个媒体错误，得到 %d，期望 %d", i, got, want[i])
		}
	}
}

func TestLocateTimelineWindow_DefaultOrderAndListBefore(t *testing.T) {
	db := newTestDB(t)
	photos := makeTimelinePhotos(t, db, 8)

	located, err := db.LocateTimelineWindow(storage.LocateTimelineParams{
		UserID:  1,
		PhotoID: photos[4].ID,
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("定位时间线失败: %v", err)
	}
	if located == nil {
		t.Fatal("应返回定位窗口")
	}
	assertPhotoIDs(t, located.Photos, []int64{
		photos[6].ID, photos[5].ID, photos[4].ID, photos[3].ID, photos[2].ID,
	})
	if located.TargetIndex != 2 {
		t.Fatalf("目标索引错误，得到 %d，期望 2", located.TargetIndex)
	}
	if located.MissingBeforeCount != 1 {
		t.Fatalf("窗口前缺口数量错误，得到 %d，期望 1", located.MissingBeforeCount)
	}
	if !located.HasBefore || !located.HasAfter {
		t.Fatalf("定位窗口前后更多标记错误: before=%v after=%v", located.HasBefore, located.HasAfter)
	}

	before1, err := db.ListPhotosBefore(storage.LocateTimelineParams{
		UserID:  1,
		PhotoID: located.Photos[0].ID,
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("查询第一段前置页面失败: %v", err)
	}
	assertPhotoIDs(t, before1.Photos, []int64{photos[7].ID})
	if before1.HasMore {
		t.Fatal("第一段前置页面不应还有更多")
	}

	before2, err := db.ListPhotosBefore(storage.LocateTimelineParams{
		UserID:  1,
		PhotoID: photos[7].ID,
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("查询第二段前置页面失败: %v", err)
	}
	if len(before2.Photos) != 0 {
		t.Fatalf("最前面的媒体不应再有前置页面，得到 %d 条", len(before2.Photos))
	}
}

func TestLocateTimelineWindow_ReverseOrderAndListBefore(t *testing.T) {
	db := newTestDB(t)
	photos := makeTimelinePhotos(t, db, 8)

	located, err := db.LocateTimelineWindow(storage.LocateTimelineParams{
		UserID:  1,
		PhotoID: photos[4].ID,
		Limit:   2,
		Reverse: true,
	})
	if err != nil {
		t.Fatalf("正序定位时间线失败: %v", err)
	}
	if located == nil {
		t.Fatal("应返回定位窗口")
	}
	assertPhotoIDs(t, located.Photos, []int64{
		photos[2].ID, photos[3].ID, photos[4].ID, photos[5].ID, photos[6].ID,
	})
	if located.TargetIndex != 2 {
		t.Fatalf("目标索引错误，得到 %d，期望 2", located.TargetIndex)
	}
	if located.MissingBeforeCount != 2 {
		t.Fatalf("正序窗口前缺口数量错误，得到 %d，期望 2", located.MissingBeforeCount)
	}
	if !located.HasBefore || !located.HasAfter {
		t.Fatalf("定位窗口前后更多标记错误: before=%v after=%v", located.HasBefore, located.HasAfter)
	}

	before1, err := db.ListPhotosBefore(storage.LocateTimelineParams{
		UserID:  1,
		PhotoID: located.Photos[0].ID,
		Limit:   2,
		Reverse: true,
	})
	if err != nil {
		t.Fatalf("查询正序第一段前置页面失败: %v", err)
	}
	assertPhotoIDs(t, before1.Photos, []int64{photos[0].ID, photos[1].ID})
	if before1.HasMore {
		t.Fatal("正序第一段前置页面不应还有更多")
	}
}

func TestLocateTimelineWindowCountsOnlyUnloadedPrefix(t *testing.T) {
	db := newTestDB(t)
	photos := makeTimelinePhotos(t, db, 8)
	for _, reverse := range []bool{false, true} {
		for rank := range photos {
			i := len(photos) - 1 - rank
			if reverse {
				i = rank
			}
			located, err := db.LocateTimelineWindow(storage.LocateTimelineParams{
				UserID: 1, PhotoID: photos[i].ID, Limit: 2, Reverse: reverse,
			})
			if err != nil || located == nil {
				t.Fatalf("reverse=%v rank=%d: locate failed: %v", reverse, rank, err)
			}
			want := max(0, rank-2)
			if located.MissingBeforeCount != want || located.HasBefore != (want > 0) {
				t.Fatalf("reverse=%v rank=%d: missing=%d before=%v, want missing=%d", reverse, rank, located.MissingBeforeCount, located.HasBefore, want)
			}
			if located.MissingBeforeCount+located.TargetIndex != rank {
				t.Fatalf("reverse=%v rank=%d: target was shifted by duplicate placeholders", reverse, rank)
			}
		}
	}
}
